package jira

import (
	"fmt"

	"github.com/dolaszy/glue/internal/github"
	"github.com/dolaszy/glue/internal/jira"
	"github.com/spf13/cobra"
)

// SyncCmd represents the jira sync command
var SyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Synchronize GitHub issues with JIRA",
	Long: `This command synchronizes GitHub issues with a JIRA board.
It creates JIRA tickets for any GitHub issues that don't have the 'glued' label,
using the issue's title and description.`,
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

		fmt.Printf("Synchronizing GitHub repository '%s' with JIRA board '%s'\n", repository, board)

		// Initialize clients
		githubClient := github.NewClient()
		jiraClient := jira.NewClient()

		// Get GitHub issues without the 'glued' label
		issues, err := githubClient.GetUnglued(repository)
		if err != nil {
			return fmt.Errorf("failed to fetch GitHub issues: %v", err)
		}

		fmt.Printf("Found %d GitHub issues to sync\n", len(issues))

		// Create JIRA tickets for each GitHub issue
		for _, issue := range issues {
			fmt.Printf("Creating JIRA ticket for GitHub issue #%d: %s\n", issue.Number, issue.Title)

			// Create JIRA ticket
			ticketID, err := jiraClient.CreateTicket(board, issue)
			if err != nil {
				fmt.Printf("Error creating JIRA ticket for issue #%d: %v\n", issue.Number, err)
				continue
			}

			fmt.Printf("Created JIRA ticket %s for GitHub issue #%d\n", ticketID, issue.Number)

			// Add 'glued' label to GitHub issue
			if err := githubClient.AddGluedLabel(repository, issue.Number); err != nil {
				fmt.Printf("Warning: Failed to add 'glued' label to GitHub issue #%d: %v\n", issue.Number, err)
			}
		}

		fmt.Println("Synchronization complete")
		return nil
	},
}
