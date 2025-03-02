package cmd

import (
	"fmt"
	"strings"
	"regexp"
	"strconv"

	"github.com/danielolaszy/glue/internal/github"
	"github.com/danielolaszy/glue/internal/jira"
	"github.com/spf13/cobra"
	"github.com/danielolaszy/glue/internal/logging"
	"github.com/danielolaszy/glue/internal/config"
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
	logging.Info("starting github to jira synchronization", 
		"repository", repository)
	
	// Initialize clients
	githubClient, err := github.NewClient()
	if err != nil {
		return 0, fmt.Errorf("failed to initialize github client: %w", err)
	}
	
	jiraClient, err := jira.NewClient()
	if err != nil {
		return 0, fmt.Errorf("failed to initialize jira client: %w", err)
	}

	// Get the GitHub domain from config
	config, err := config.LoadConfig()
	if err != nil {
		logging.Warn("failed to load config for GitHub domain, using default", "error", err)
	}
	
	// Default to github.com if not specified
	gitHubDomain := "github.com"
	if config != nil && config.GitHub.Domain != "" {
		gitHubDomain = config.GitHub.Domain
	}
	
	logging.Debug("using github domain for issue parsing", "domain", gitHubDomain)

	// First pass: Create all tickets (existing code)
	syncCount, err := createJiraTickets(repository, githubClient, jiraClient)
	if err != nil {
		return syncCount, err
	}
	
	// Second pass: Establish hierarchies
	hierarchyCount, err := establishHierarchies(repository, githubClient, jiraClient, gitHubDomain)
	if err != nil {
		logging.Error("error establishing hierarchies", "error", err)
		// Continue anyway, we've created the tickets at least
	} else {
		logging.Info("established hierarchical relationships", "count", hierarchyCount)
	}
	
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
		if len(childLinks) == 0 {
			logging.Debug("feature issue has no child issues",
				"repository", repository,
				"issue_number", issue.Number)
			continue
		}
		
		logging.Info("found child issues in feature description",
			"repository", repository,
			"issue_number", issue.Number,
			"child_count", len(childLinks))
		
		// Process each child issue
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
			
			// Create the parent-child link in JIRA
			err = jiraClient.CreateParentChildLink(parentJiraID, childJiraID)
			if err != nil {
				logging.Error("failed to create parent-child link in jira",
					"parent", parentJiraID,
					"child", childJiraID,
					"error", err)
				continue
			}
			
			logging.Info("created parent-child link in jira",
				"parent", parentJiraID,
				"child", childJiraID)
			linkCount++
		}
	}
	
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
		
		logging.Info("found jira-project label", 
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
		
		storyTypeID, err := jiraClient.GetIssueTypeID(jiraProjectKey, "story")
		if err != nil {
			logging.Warn("story issue type not found, will use feature type", 
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
