// Package cmd provides the command-line interface for the Glue CLI tool.
package cmd

import (
	"fmt"
	"log"
	"strings"

	"github.com/danielolaszy/glue/internal/github"
	"github.com/danielolaszy/glue/internal/jira"
	"github.com/spf13/cobra"
)

// jiraCmd represents the command to synchronize GitHub issues with JIRA.
// It creates JIRA tickets for GitHub issues that aren't already linked to the specified project.
var jiraCmd = &cobra.Command{
	Use:   "jira",
	Short: "Synchronize GitHub with JIRA",
	Long: `Synchronize GitHub issues with a JIRA board.

This command will create JIRA tickets for any GitHub issues that aren't
already linked to the specified JIRA project. It adds a 'jira-id: PROJECT-123'
label to GitHub issues that have been synchronized.

Issues will be categorized based on their existing labels:
- GitHub issues with a 'type: feature' label will be created as 'Feature' type in JIRA
- GitHub issues with a 'type: story' label will be created as 'Story' type in JIRA
- Other GitHub issues will be created as 'Task' type in JIRA by default`,
	RunE: func(cmd *cobra.Command, args []string) error {
		repository, err := cmd.Flags().GetString("repository")
		if err != nil {
			return err
		}

		board, err := cmd.Flags().GetString("board")
		if err != nil {
			return err
		}

		if repository == "" {
			return fmt.Errorf("repository flag is required")
		}

		if board == "" {
			return fmt.Errorf("board flag is required")
		}

		fmt.Printf("Synchronizing GitHub repository '%s' with JIRA project '%s'\n", repository, board)

		// Perform synchronization
		syncCount, err := syncGitHubToJira(repository, board)
		if err != nil {
			log.Printf("Synchronization error: %v", err)
			return err
		}

		fmt.Printf("\nSynchronization complete: %d issues synchronized with JIRA project '%s'\n", syncCount, board)

		return nil
	},
}

// init is called when the package is initialized.
// It adds JIRA-specific flags to the jira command.
func init() {
	// Add board flag that's specific to JIRA command
	jiraCmd.Flags().StringP("board", "b", "", "JIRA board/project key")
}

// syncGitHubToJira synchronizes GitHub issues with JIRA tickets.
//
// Parameters:
//   - repository: The GitHub repository name (e.g., "username/repo")
//   - jiraProjectKey: The JIRA project key (e.g., "ABC" for tickets like "ABC-123")
//
// Returns:
//   - The number of issues successfully synchronized
//   - An error if the synchronization process failed
func syncGitHubToJira(repository, jiraProjectKey string) (int, error) {
	// Initialize clients
	githubClient, err := github.NewClient()
	if err != nil {
		return 0, fmt.Errorf("failed to initialize GitHub client: %w", err)
	}
	jiraClient, err := jira.NewClient()
	if err != nil {
		return 0, fmt.Errorf("failed to initialize JIRA client: %w", err)
	}

	// Get all GitHub issues
	issues, err := githubClient.GetAllIssues(repository)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch GitHub issues: %v", err)
	}

	fmt.Printf("Found %d GitHub issues to process\n", len(issues))
	syncCount := 0

	for _, issue := range issues {
		// Check if issue already has a JIRA ID label for this project
		jiraIDPrefix := fmt.Sprintf("jira-id: %s-", jiraProjectKey)
		hasProjectLabel := false

		labels, err := githubClient.GetLabelsForIssue(repository, issue.Number)
		if err != nil {
			log.Printf("Error fetching labels for issue #%d: %v", issue.Number, err)
			continue
		}

		for _, label := range labels {
			if strings.HasPrefix(label, jiraIDPrefix) {
				hasProjectLabel = true
				break
			}
		}

		if hasProjectLabel {
			// Issue already synchronized with this project
			log.Printf("Issue #%d already synchronized with JIRA project %s, skipping",
				issue.Number, jiraProjectKey)
			continue
		}

		// Determine issue type based on existing labels
		issueType := "Task" // Default type
		for _, label := range labels {
			if label == "type: feature" {
				issueType = "Feature"
				break
			} else if label == "type: story" {
				issueType = "Story"
				break
			}
		}

		// Create JIRA ticket
		fmt.Printf("Creating JIRA ticket for GitHub issue #%d: %s\n", issue.Number, issue.Title)
		ticketID, err := jiraClient.CreateTicket(jiraProjectKey, issue, issueType)
		if err != nil {
			log.Printf("Error creating JIRA ticket for issue #%d: %v", issue.Number, err)
			continue
		}

		// Add JIRA ID label - GitHub will automatically create it if it doesn't exist
		labelToAdd := fmt.Sprintf("jira-id: %s", ticketID)
		err = githubClient.AddLabels(repository, issue.Number, labelToAdd)
		if err != nil {
			log.Printf("Warning: Failed to add JIRA ID label to GitHub issue #%d: %v",
				issue.Number, err)
		}

		fmt.Printf("Created JIRA ticket %s for GitHub issue #%d\n", ticketID, issue.Number)
		syncCount++
	}

	return syncCount, nil
}
