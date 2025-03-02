// Package cmd provides the command-line interface for the Glue CLI tool.
package cmd

import (
	"fmt"
	"strings"

	"github.com/danielolaszy/glue/internal/github"
	"github.com/danielolaszy/glue/internal/jira"
	"github.com/spf13/cobra"
	"github.com/danielolaszy/glue/internal/logging"
)

// jiraCmd represents the command to synchronize GitHub issues with JIRA.
// It creates JIRA tickets for GitHub issues that aren't already linked to the specified project.
var jiraCmd = &cobra.Command{
	Use:   "jira",
	Short: "Synchronize GitHub with JIRA",
	Long: `Synchronize GitHub issues with a JIRA board.

This command will create JIRA tickets for any GitHub issues that aren't
already linked to a JIRA project. It adds a 'jira-id: PROJECT-123'
label to GitHub issues that have been synchronized.

Every GitHub issue must have a 'jira-project: BOARD_NAME' label to specify
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

// init is called when the package is initialized.
// It adds JIRA-specific flags to the jira command.
func init() {
	// No JIRA-specific flags needed as we use GitHub labels
}

// extractJiraProject checks for a label beginning with "jira-project:" and
// extracts the JIRA project name from it
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

// syncGitHubToJira synchronizes GitHub issues with JIRA tickets.
//
// Parameters:
//   - repository: The GitHub repository name (e.g., "username/repo")
//
// Returns:
//   - The number of issues successfully synchronized
//   - An error if the synchronization process failed
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
		
		// Check if the issue has a jira-project label to determine which board to use
		jiraProjectKey, found := extractJiraProject(labels)
		if !found {
			logging.Warn("skipping issue without jira-project label", 
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
			logging.Error("failed to get feature type ID for project", 
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

// hasLabel checks if a specific label is present in a slice of labels.
func hasLabel(labels []string, targetLabel string) bool {
	for _, label := range labels {
		if strings.EqualFold(label, targetLabel) {
			return true
		}
	}
	return false
}
