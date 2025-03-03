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
// It creates JIRA tickets for GitHub issues labeled with the specified JIRA project(s).
var jiraCmd = &cobra.Command{
	Use:   "jira",
	Short: "Synchronize GitHub with JIRA",
	Long: `Synchronize GitHub issues with JIRA boards.

This command will create JIRA tickets for GitHub issues labeled with the specified JIRA project(s).
You can specify multiple boards using -b/--board flag multiple times.

Example:
  glue jira -r owner/repo -b PROJ1 -b PROJ2

Issues will be categorized based on their labels:
- GitHub issues with a 'feature' label will be created as 'Feature' type in JIRA
- GitHub issues with a 'story' label will be created as 'Story' type in JIRA
- Other GitHub issues will be created as 'Story' type in JIRA by default`,
	RunE: func(cmd *cobra.Command, args []string) error {
		repository, err := cmd.Flags().GetString("repository")
		if err != nil {
			return err
		}

		boards, err := cmd.Flags().GetStringArray("board")
		if err != nil {
			return err
		}

		if repository == "" {
			return fmt.Errorf("repository flag is required")
		}

		if len(boards) == 0 {
			return fmt.Errorf("at least one JIRA board must be specified using --board")
		}

		logging.Info("starting synchronization",
			"repository", repository,
			"boards", boards)

		// Initialize clients once, outside the loop
		githubClient, err := github.NewClient()
		if err != nil {
			return fmt.Errorf("failed to initialize github client: %v", err)
		}

		jiraClient, err := jira.NewClient()
		if err != nil {
			return fmt.Errorf("failed to initialize jira client: %v", err)
		}

		// Process each board
		totalSynced := 0
		for _, board := range boards {
			logging.Info("processing board", "board", board)
			
			// Get issues labeled with this board
			syncCount, err := createJiraTickets(repository, board, githubClient, jiraClient)
			if err != nil {
				logging.Error("error processing board",
					"board", board,
					"error", err)
				continue
			}

			// Establish hierarchies for this board
			linkCount, err := establishHierarchies(repository, board, githubClient, jiraClient)
			if err != nil {
				logging.Error("error establishing hierarchies for board",
					"board", board,
					"error", err)
			}

			// Sync closed issues for this board
			closeCount, err := syncClosedIssues(repository, board, githubClient, jiraClient)
			if err != nil {
				logging.Error("error syncing closed issues for board",
					"board", board,
					"error", err)
			}

			logging.Info("completed processing board",
				"board", board,
				"tickets_created", syncCount,
				"links_created", linkCount,
				"tickets_closed", closeCount)

			totalSynced += syncCount
		}

		logging.Info("synchronization complete",
			"total_synchronized", totalSynced,
			"boards_processed", len(boards))

		return nil
	},
}

func init() {
	rootCmd.AddCommand(jiraCmd)
	jiraCmd.Flags().StringArrayP("board", "b", []string{}, "JIRA project board(s) to sync with (can be specified multiple times)")
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

// syncGitHubToJira synchronizes GitHub issues with JIRA tickets for a specific board
func syncGitHubToJira(repository string, board string) (int, error) {
	logging.Info("syncing github to jira",
		"repository", repository,
		"board", board)

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

	// Create Jira tickets for GitHub issues
	syncCount, err := createJiraTickets(repository, board, githubClient, jiraClient)
	if err != nil {
		return 0, fmt.Errorf("error creating jira tickets: %v", err)
	}

	// Establish hierarchies between related tickets
	linkCount, err := establishHierarchies(repository, board, githubClient, jiraClient)
	if err != nil {
		logging.Error("error establishing hierarchies", "error", err)
	}

	// Synchronize closed issues
	closeCount, err := syncClosedIssues(repository, board, githubClient, jiraClient)
	if err != nil {
		logging.Error("error synchronizing closed issues", "error", err)
	}

	logging.Info("github to jira sync completed",
		"repository", repository,
		"board", board,
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

// createJiraTickets creates JIRA tickets for GitHub issues with the specified board label
func createJiraTickets(repository string, board string, githubClient *github.Client, jiraClient *jira.Client) (int, error) {
	// Get GitHub issues filtered by board label
	issues, err := githubClient.GetIssuesWithLabel(repository, board)
	if err != nil {
		logging.Error("failed to fetch github issues", "error", err)
		return 0, fmt.Errorf("failed to fetch github issues: %v", err)
	}

	logging.Info("found github issues",
		"count", len(issues),
		"repository", repository,
		"board", board)

	syncCount := 0
	for _, issue := range issues {
		// Check if issue already has a JIRA ID in its title
		if hasJiraIDPrefix(issue.Title) {
			logging.Debug("issue already has JIRA ID, skipping",
				"issue_number", issue.Number,
				"title", issue.Title)
			continue
		}

		// Get issue type IDs for the project
		featureTypeID, err := jiraClient.GetIssueTypeID(board, "feature")
		if err != nil {
			logging.Error("failed to get 'feature' type ID",
				"board", board,
				"error", err)
			continue
		}

		// Determine issue type based on labels
		issueTypeID := featureTypeID // default to feature type
		if hasLabel(issue.Labels, "feature") {
			issueTypeID = featureTypeID
		} else if hasLabel(issue.Labels, "story") {
			storyTypeID, err := jiraClient.GetIssueTypeID(board, "story")
			if err == nil {
				issueTypeID = storyTypeID
			}
		}

		// Create JIRA ticket
		ticketID, err := jiraClient.CreateTicketWithTypeID(board, issue, issueTypeID)
		if err != nil {
			logging.Error("error creating jira ticket",
				"repository", repository,
				"issue_number", issue.Number,
				"board", board,
				"error", err)
			continue
		}

		// Update GitHub issue title with JIRA ID
		newTitle := fmt.Sprintf("[%s] %s", ticketID, issue.Title)
		err = githubClient.UpdateIssueTitle(repository, issue.Number, newTitle)
		if err != nil {
			logging.Error("failed to update github issue title",
				"repository", repository,
				"issue_number", issue.Number,
				"error", err)
			continue
		}

		logging.Info("created jira ticket and updated github title",
			"jira_ticket_id", ticketID,
			"board", board,
			"repository", repository,
			"issue_number", issue.Number)
		syncCount++
	}

	return syncCount, nil
}

// hasJiraIDPrefix checks if an issue title starts with a JIRA ID pattern [ABC-123]
func hasJiraIDPrefix(title string) bool {
	return regexp.MustCompile(`^\[[A-Z]+-\d+\]`).MatchString(title)
}

// extractJiraIDFromTitle extracts the JIRA ID from a title if present
func extractJiraIDFromTitle(title string) (string, bool) {
	matches := regexp.MustCompile(`^\[([A-Z]+-\d+)\]`).FindStringSubmatch(title)
	if len(matches) > 1 {
		return matches[1], true
	}
	return "", false
}

// syncClosedIssues checks all GitHub issues with jira-id labels that are closed
// and closes their corresponding JIRA tickets. It returns the number of
// JIRA tickets closed and any error encountered during the process.
func syncClosedIssues(repository string, board string, githubClient *github.Client, jiraClient *jira.Client) (int, error) {
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

// establishHierarchies creates parent-child relationships in JIRA based on GitHub issue links
func establishHierarchies(repository string, board string, githubClient *github.Client, jiraClient *jira.Client) (int, error) {
	// Get GitHub domain from config
	config, err := config.LoadConfig()
	if err != nil {
		return 0, fmt.Errorf("failed to load configuration: %v", err)
	}
	gitHubDomain := config.GitHub.Domain
	if gitHubDomain == "" {
		gitHubDomain = "github.com" // Default fallback
	}

	// Get all GitHub issues with the board label
	issues, err := githubClient.GetIssuesWithLabel(repository, board)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch github issues: %v", err)
	}

	linkCount := 0

	// Find feature issues
	for _, issue := range issues {
		// Only process feature issues
		if !hasLabel(issue.Labels, "feature") {
			continue
		}

		// Find the JIRA ID from the title
		parentJiraID, found := extractJiraIDFromTitle(issue.Title)
		if !found {
			logging.Warn("feature issue has no JIRA ID in title, skipping hierarchy",
				"repository", repository,
				"issue_number", issue.Number)
			continue
		}

		// Parse the child issues from the description using configured GitHub domain
		childLinks := parseChildIssues(issue.Description, gitHubDomain)
		if len(childLinks) == 0 {
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

			// Get the child issue to check its title
			childIssue, err := githubClient.GetIssue(childRepo, childIssueNum)
			if err != nil {
				logging.Error("failed to fetch child issue",
					"repository", childRepo,
					"issue_number", childIssueNum,
					"error", err)
				continue
			}

			// Extract JIRA ID from child issue title
			childJiraID, found := extractJiraIDFromTitle(childIssue.Title)
			if !found {
				logging.Debug("child issue has no JIRA ID in title, skipping link",
					"repository", childRepo,
					"issue_number", childIssueNum)
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

	return linkCount, nil
}

// parseChildIssues extracts GitHub issue links from the "## Issues" section
// of a GitHub issue description.
func parseChildIssues(description string, gitHubDomain string) []string {
	var childLinks []string

	// Find the "## Issues" section
	issuesSection := findIssuesSection(description)
	if issuesSection == "" {
		return childLinks
	}

	// Escape dots and other special characters in the domain
	escapedDomain := regexp.QuoteMeta(gitHubDomain)
	pattern := fmt.Sprintf(`https://%s/[^/]+/[^/]+/issues/\d+`, escapedDomain)
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

// parseGitHubIssueLink extracts repository and issue number from a GitHub issue URL
func parseGitHubIssueLink(link string, gitHubDomain string) (string, int, error) {
	// Escape dots and other special characters in the domain
	escapedDomain := regexp.QuoteMeta(gitHubDomain)
	pattern := fmt.Sprintf(`https://%s/([^/]+/[^/]+)/issues/(\d+)`, escapedDomain)
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
