package cmd

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/danielolaszy/glue/internal/config"
	"github.com/danielolaszy/glue/internal/github"
	"github.com/danielolaszy/glue/internal/jira"
	"github.com/danielolaszy/glue/internal/logging"
	"github.com/spf13/cobra"
)

// jiraCmd represents the command to synchronize GitHub issues with JIRA.
// It creates JIRA tickets for GitHub issues that aren't already linked to the specified project.
var jiraCmd = &cobra.Command{
	Use:   "jira",
	Short: "Synchronize GitHub with JIRA",
	Long: `Synchronize GitHub issues with a JIRA board.

This command will create JIRA tickets for any GitHub issues that aren't
already linked to a JIRA project. It adds a 'jira-id: <JIRA_ID>'
label to GitHub issues that have been synchronized.

Every GitHub issue must have a 'jira-project: <BOARD_NAME>' label to specify
which JIRA board the issue should be created on.

Issues will be categorized based on their existing labels:
- GitHub issues with a 'type: feature' label will be created as 'Feature' type in JIRA
- GitHub issues with a 'type: story' label will be created as 'Story' type in JIRA
- Other GitHub issues will be created as 'Task' type in JIRA by default`,
	RunE: func(cmd *cobra.Command, args []string) error {
		repository, err := cmd.Flags().GetString("repository")
		if err != nil {
			return err
		}

		if repository == "" {
			return fmt.Errorf("repository flag is required")
		}

		// Perform synchronization
		syncCount, err := syncGitHubToJira(repository)
		if err != nil {
			logging.Error("synchronization error", "error", err)
			return err
		}
		logging.Info("synchronization complete",
			"synchronized_count", syncCount,
		)
		return nil
	},
}

// extractJiraProject searches for a label beginning with "jira-project:" and
// extracts the JIRA project name from it. It returns the project name and a
// boolean indicating whether the label was found.
func extractJiraProject(labels []string) (string, bool) {
	const prefix = "jira-project:"

	for _, label := range labels {
		if strings.HasPrefix(label, prefix) {
			// Extract the project name, which is after the prefix and a space
			parts := strings.SplitN(label, ":", 2)
			if len(parts) == 2 {
				boardName := strings.TrimSpace(parts[1])
				if boardName != "" {
					return boardName, true
				}
			}
		}
	}

	return "", false
}

// getJiraIDFromLabels searches for a label beginning with "jira-id:" and
// extracts the JIRA ticket ID from it. It returns the ticket ID if found
// or an empty string if no matching label exists.
func getJiraIDFromLabels(labels []string) string {
	const prefix = "jira-id:"

	for _, label := range labels {
		if strings.HasPrefix(label, prefix) {
			// Extract the JIRA ID, which is after the prefix and a space
			parts := strings.SplitN(label, ":", 2)
			if len(parts) == 2 {
				jiraID := strings.TrimSpace(parts[1])
				if jiraID != "" {
					return jiraID
				}
			}
		}
	}

	return ""
}

// syncGitHubToJira synchronizes GitHub issues with JIRA tickets. It initializes
// the required clients, creates JIRA tickets for GitHub issues with appropriate labels,
// and establishes hierarchical relationships between related issues. It returns the
// number of issues synchronized and any error encountered during the process.
func syncGitHubToJira(repository string) (int, error) {
	logging.Info("syncing github to jira", "repository", repository)

	// Initialize GitHub client
	githubClient, err := github.NewClient()
	if err != nil {
		return 0, fmt.Errorf("failed to initialize github client: %v", err)
	}

	// Initialize JIRA client
	jiraClient, err := jira.NewClient()
	if err != nil {
		return 0, fmt.Errorf("failed to initialize jira client: %v", err)
	}

	// Get GitHub domain from config, default to github.com
	config, err := config.LoadConfig()
	if err != nil {
		return 0, fmt.Errorf("failed to load configuration: %v", err)
	}
	gitHubDomain := config.GitHub.Domain
	if gitHubDomain == "" {
		gitHubDomain = "github.com"
	}

	// Create Jira tickets for GitHub issues
	syncCount, err := createJiraTickets(repository, githubClient, jiraClient)
	if err != nil {
		return 0, fmt.Errorf("error creating jira tickets: %v", err)
	}

	// Establish hierarchies between related tickets
	linkCount, err := establishHierarchies(repository, githubClient, jiraClient, gitHubDomain)
	if err != nil {
		logging.Error("error establishing hierarchies", "error", err)
		// Continue anyway to show the sync count
	}

	// Synchronize closed issues
	closeCount, err := syncClosedIssues(repository, githubClient, jiraClient)
	if err != nil {
		logging.Error("error synchronizing closed issues", "error", err)
		// Continue anyway to show the sync count
	}

	logging.Info("github to jira sync completed",
		"repository", repository,
		"tickets_created", syncCount,
		"links_created", linkCount,
		"tickets_closed", closeCount)

	return syncCount, nil
}

// hasLabel checks if a specific label exists in a slice of labels using
// case-insensitive comparison. It returns true if the label is found.
func hasLabel(labels []string, targetLabel string) bool {
	for _, label := range labels {
		if strings.EqualFold(label, targetLabel) {
			return true
		}
	}
	return false
}

// establishHierarchies creates parent-child relationships in JIRA based on
// GitHub issue links in feature issue descriptions. It processes issues with the
// "type: feature" label and links them to their child issues in JIRA.
// It returns the number of relationships created and any error encountered.
func establishHierarchies(repository string, githubClient *github.Client, jiraClient *jira.Client, gitHubDomain string) (int, error) {
	// Get all GitHub issues
	issues, err := githubClient.GetAllIssues(repository)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch github issues: %v", err)
	}

	linkCount := 0
	unlinkCount := 0

	// Find feature issues
	for _, issue := range issues {
		labels, err := githubClient.GetLabelsForIssue(repository, issue.Number)
		if err != nil {
			logging.Error("failed to fetch labels for issue",
				"repository", repository,
				"issue_number", issue.Number,
				"error", err)
			continue
		}

		// Only process feature issues
		if !hasLabel(labels, "type: feature") {
			continue
		}

		// Find the JIRA ID for this feature
		parentJiraID, found := findJiraIDFromLabels(labels)
		if !found {
			logging.Warn("feature issue has no JIRA ID, skipping hierarchy",
				"repository", repository,
				"issue_number", issue.Number)
			continue
		}

		// Parse the child issues from the description
		childLinks := parseChildIssues(issue.Description, gitHubDomain)

		// Create a map of GitHub child issues
		githubChildrenMap := make(map[string]bool)
		githubChildJiraIDs := make([]string, 0)

		// Process each child issue found in GitHub
		for _, link := range childLinks {
			childRepo, childIssueNum, err := parseGitHubIssueLink(link, gitHubDomain)
			if err != nil {
				logging.Error("failed to parse child issue link",
					"link", link,
					"error", err)
				continue
			}

			// Get the child issue's labels
			childLabels, err := githubClient.GetLabelsForIssue(childRepo, childIssueNum)
			if err != nil {
				logging.Error("failed to fetch labels for child issue",
					"repository", childRepo,
					"issue_number", childIssueNum,
					"error", err)
				continue
			}

			// Find the JIRA ID for the child issue
			childJiraID, found := findJiraIDFromLabels(childLabels)
			if !found {
				logging.Debug("child issue has no 'jira-id' label, skipping link",
					"repository", childRepo,
					"issue_number", childIssueNum)
				continue
			}

			// Add to our tracking maps
			githubChildrenMap[childJiraID] = true
			githubChildJiraIDs = append(githubChildJiraIDs, childJiraID)
		}

		// Get existing linked issues in JIRA
		jiraLinkedIssues, err := jiraClient.GetLinkedIssues(parentJiraID)
		if err != nil {
			logging.Error("failed to get linked issues from JIRA",
				"parent", parentJiraID,
				"error", err)
			// Continue anyway to at least create new links
		}

		// Track which issues need to be unlinked (found in JIRA but not in GitHub description)
		if len(jiraLinkedIssues) > 0 {
			logging.Debug("found existing linked issues in JIRA",
				"parent", parentJiraID,
				"count", len(jiraLinkedIssues))

			for _, jiraChildID := range jiraLinkedIssues {
				if _, exists := githubChildrenMap[jiraChildID]; !exists {
					// This issue is linked in JIRA but not in GitHub description
					logging.Info("detected removed issue link in GitHub",
						"parent", parentJiraID,
						"child", jiraChildID)

					// Unlink the issue
					err := jiraClient.DeleteIssueLink(parentJiraID, jiraChildID)
					if err != nil {
						logging.Error("failed to remove parent-child link in JIRA",
							"parent", parentJiraID,
							"child", jiraChildID,
							"error", err)
						continue
					}

					logging.Info("successfully removed parent-child link in JIRA",
						"parent", parentJiraID,
						"child", jiraChildID)
					unlinkCount++
				}
			}
		}

		// If no child issues in GitHub description, log and continue to next feature
		if len(childLinks) == 0 {
			logging.Debug("feature issue has no child issues",
				"repository", repository,
				"issue_number", issue.Number)
			continue
		}

		logging.Info("found child issues in feature description",
			"repository", repository,
			"issue_number", issue.Number,
			"child_count", len(githubChildJiraIDs))

		// Now process each child issue to create links that don't exist
		for _, childJiraID := range githubChildJiraIDs {
			// Check if the link already exists in JIRA
			exists, err := jiraClient.CheckParentChildLinkExists(parentJiraID, childJiraID)
			if err != nil {
				logging.Error("failed to check if parent-child link exists in JIRA",
					"parent", parentJiraID,
					"child", childJiraID,
					"error", err)
				continue
			}

			if exists {
				logging.Debug("parent-child link already exists in JIRA, skipping",
					"parent", parentJiraID,
					"child", childJiraID)
				continue
			}

			// Create the parent-child link in JIRA
			err = jiraClient.CreateParentChildLink(parentJiraID, childJiraID)
			if err != nil {
				logging.Error("failed to create parent-child link in JIRA",
					"parent", parentJiraID,
					"child", childJiraID,
					"error", err)
				continue
			}

			logging.Info("successfully created parent-child link in JIRA",
				"parent", parentJiraID,
				"child", childJiraID)
			linkCount++
		}
	}

	logging.Info("hierarchy synchronization complete",
		"links_created", linkCount,
		"links_removed", unlinkCount)

	return linkCount, nil
}

// parseChildIssues extracts GitHub issue links from the "## Issues" section
// of a GitHub issue description. It returns a slice of GitHub issue URLs.
// The gitHubDomain parameter allows for custom GitHub Enterprise domains.
func parseChildIssues(description string, gitHubDomain string) []string {
	var childLinks []string

	// Find the "## Issues" section
	issuesSection := findIssuesSection(description)
	if issuesSection == "" {
		return childLinks
	}

	// Extract GitHub issue links using regex with the provided domain
	pattern := fmt.Sprintf(`https://%s/[^/]+/[^/]+/issues/\d+`, regexp.QuoteMeta(gitHubDomain))
	re := regexp.MustCompile(pattern)
	matches := re.FindAllString(issuesSection, -1)

	return matches
}

// findIssuesSection extracts the content of the "## Issues" section from a description.
// It returns the text between "## Issues" and the next section header, if found.
func findIssuesSection(description string) string {
	// Split the description by "## Issues" and take the content after it
	parts := strings.Split(description, "## Issues")
	if len(parts) < 2 {
		return ""
	}

	// Find the next section header or return the rest of the content
	nextSectionIdx := strings.Index(parts[1], "## ")
	if nextSectionIdx != -1 {
		return parts[1][:nextSectionIdx]
	}

	return parts[1]
}

// parseGitHubIssueLink extracts repository and issue number from a GitHub issue URL.
// It returns the repository in the format "owner/repo" and the issue number, or
// an error if the URL format is invalid.
// The gitHubDomain parameter allows for custom GitHub Enterprise domains.
func parseGitHubIssueLink(link string, gitHubDomain string) (string, int, error) {
	pattern := fmt.Sprintf(`https://%s/([^/]+/[^/]+)/issues/(\d+)`, regexp.QuoteMeta(gitHubDomain))
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(link)

	if len(matches) != 3 {
		return "", 0, fmt.Errorf("invalid GitHub issue link format: %s", link)
	}

	repo := matches[1]
	issueNum, err := strconv.Atoi(matches[2])
	if err != nil {
		return "", 0, fmt.Errorf("failed to parse issue number: %v", err)
	}

	return repo, issueNum, nil
}

// findJiraIDFromLabels searches for a "jira-id:" label and extracts the JIRA ticket ID.
// It returns the JIRA ID (e.g., "PROJECT-123") and whether a matching label was found.
func findJiraIDFromLabels(labels []string) (string, bool) {
	jiraIDPattern := regexp.MustCompile(`^jira-id: ([A-Z]+-\d+)$`)

	for _, label := range labels {
		matches := jiraIDPattern.FindStringSubmatch(label)
		if len(matches) == 2 {
			return matches[1], true
		}
	}

	return "", false
}

// createJiraTickets creates JIRA tickets for GitHub issues with appropriate labels.
// It determines the issue type based on GitHub labels, creates the corresponding
// JIRA ticket, and adds a reference label back to the GitHub issue.
// It returns the number of tickets created and any error encountered.
func createJiraTickets(repository string, githubClient *github.Client, jiraClient *jira.Client) (int, error) {
	// Get all GitHub issues
	issues, err := githubClient.GetAllIssues(repository)
	if err != nil {
		logging.Error("failed to fetch github issues", "error", err)
		return 0, fmt.Errorf("failed to fetch github issues: %v", err)
	}

	logging.Info("found github issues", "count", len(issues), "repository", repository)
	syncCount := 0

	for _, issue := range issues {
		logging.Debug("checking labels for issue",
			"repository", repository,
			"issue_number", issue.Number,
		)

		labels, err := githubClient.GetLabelsForIssue(repository, issue.Number)
		if err != nil {
			logging.Error("failed to fetch labels for issue",
				"repository", repository,
				"issue_number", issue.Number,
				"error", err)
			continue
		}

		// Check if the issue has a 'jira-project:' label to determine which board to use
		jiraProjectKey, found := extractJiraProject(labels)
		if !found {
			logging.Debug("skipping issue without 'jira-project:' label",
				"repository", repository,
				"issue_number", issue.Number)
			continue
		}

		logging.Info("found 'jira-project:' label",
			"issue_number", issue.Number,
			"board", jiraProjectKey)

		// Get issue type IDs for the project
		featureTypeID, err := jiraClient.GetIssueTypeID(jiraProjectKey, "feature")
		if err != nil {
			logging.Error("failed to get 'feature' type ID for project, ensure it exists in the jira project",
				"project", jiraProjectKey,
				"issue_number", issue.Number,
				"error", err)
			continue
		}

		// Get the story type ID
		storyTypeID, err := jiraClient.GetIssueTypeID(jiraProjectKey, "story")
		if err != nil {
			logging.Warn("failed to get 'story' type ID for project, ensure it exists in the jira project",
				"project", jiraProjectKey,
				"issue_number", issue.Number)
			storyTypeID = featureTypeID
		}

		// Check if issue already has a JIRA ID label for this project
		jiraIDPrefix := fmt.Sprintf("jira-id: %s-", jiraProjectKey)
		hasProjectLabel := false

		for _, label := range labels {
			if strings.HasPrefix(label, jiraIDPrefix) {
				hasProjectLabel = true
				break
			}
		}

		if hasProjectLabel {
			// Issue already synchronized with this project
			logging.Debug("issue already synchronized, skipping",
				"repository", repository,
				"issue_number", issue.Number,
				"jira_project", jiraProjectKey,
			)
			continue
		}

		// Use the type IDs when creating tickets
		var issueTypeID string
		if hasLabel(labels, "type: feature") {
			logging.Debug("using feature type for issue", "issue_number", issue.Number)
			issueTypeID = featureTypeID
		} else if hasLabel(labels, "type: story") {
			logging.Debug("using story type for issue", "issue_number", issue.Number)
			issueTypeID = storyTypeID
		} else {
			// Default to story type
			logging.Debug("using default type for issue", "issue_number", issue.Number, "type", storyTypeID)
			issueTypeID = storyTypeID
		}

		// Create JIRA ticket
		logging.Info("creating jira ticket",
			"repository", repository,
			"issue_number", issue.Number,
			"jira_project", jiraProjectKey,
			"issue_type", issueTypeID,
		)
		ticketID, err := jiraClient.CreateTicketWithTypeID(jiraProjectKey, issue, issueTypeID)
		if err != nil {
			logging.Error("error creating jira ticket",
				"repository", repository,
				"issue_number", issue.Number,
				"jira_project", jiraProjectKey,
				"error", err,
			)
			continue
		}

		// Add JIRA ID label - GitHub will automatically create it if it doesn't exist
		labelToAdd := fmt.Sprintf("jira-id: %s", ticketID)
		err = githubClient.AddLabels(repository, issue.Number, labelToAdd)
		if err != nil {
			logging.Error("failed to add label to github issue",
				"label", labelToAdd,
				"repository", repository,
				"issue_number", issue.Number,
				"jira_project", jiraProjectKey,
				"error", err,
			)
		}

		logging.Info("created jira ticket",
			"jira_ticket_id", ticketID,
			"jira_project", jiraProjectKey,
			"repository", repository,
			"issue_number", issue.Number,
		)
		syncCount++
	}

	return syncCount, nil
}

// syncClosedIssues checks all GitHub issues with jira-id labels that are closed
// and closes their corresponding JIRA tickets. It returns the number of
// JIRA tickets closed and any error encountered during the process.
func syncClosedIssues(repository string, githubClient *github.Client, jiraClient *jira.Client) (int, error) {
	logging.Info("checking for closed github issues", "repository", repository)

	// Parse repository owner and name
	parts := strings.Split(repository, "/")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid repository format: %s, expected format: owner/repo", repository)
	}

	// Get all issues from GitHub (including closed ones)
	// Use the method we added to our client
	closedIssues, err := githubClient.GetClosedIssues(repository)
	if err != nil {
		logging.Error("failed to fetch closed github issues", "error", err)
		return 0, fmt.Errorf("failed to fetch closed GitHub issues: %v", err)
	}

	logging.Info("found closed github issues", "count", len(closedIssues), "repository", repository)
	closeCount := 0

	// Process each closed issue
	for _, issue := range closedIssues {
		// Get labels for the issue
		labels, err := githubClient.GetLabelsForIssue(repository, issue.Number)
		if err != nil {
			logging.Error("failed to fetch labels for closed issue",
				"repository", repository,
				"issue_number", issue.Number,
				"error", err)
			continue
		}

		// Check for jira-id label
		jiraID := getJiraIDFromLabels(labels)
		if jiraID == "" {
			logging.Debug("closed issue has no jira-id label, skipping",
				"repository", repository,
				"issue_number", issue.Number)
			continue
		}

		// Close the corresponding JIRA ticket
		logging.Info("closing jira ticket for closed github issue",
			"repository", repository,
			"issue_number", issue.Number,
			"jira_ticket", jiraID)

		err = jiraClient.CloseTicket(jiraID)
		if err != nil {
			logging.Error("failed to close jira ticket",
				"repository", repository,
				"issue_number", issue.Number,
				"jira_ticket", jiraID,
				"error", err)
			continue
		}

		logging.Info("successfully closed jira ticket",
			"repository", repository,
			"issue_number", issue.Number,
			"jira_ticket", jiraID)
		closeCount++
	}

	return closeCount, nil
}
