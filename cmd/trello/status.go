package trello

import (
	"fmt"

	"github.com/dolaszy/glue/internal/github"
	"github.com/dolaszy/glue/internal/trello"
	"github.com/spf13/cobra"
)

// StatusCmd represents the trello status command
var StatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check synchronization status between GitHub and Trello",
	Long: `This command displays statistics about the synchronization status
between GitHub issues and Trello cards.`,
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

		fmt.Printf("Checking synchronization status between GitHub repository '%s' and Trello board '%s'\n", repository, board)

		// Initialize clients
		githubClient := github.NewClient()
		trelloClient := trello.NewClient()

		// Get GitHub statistics
		glued, unglued, err := githubClient.GetSyncStats(repository)
		if err != nil {
			return fmt.Errorf("failed to fetch GitHub statistics: %v", err)
		}

		// Get Trello statistics
		totalCards, gluedCards, err := trelloClient.GetBoardStats(board)
		if err != nil {
			return fmt.Errorf("failed to fetch Trello statistics: %v", err)
		}

		// Display statistics
		fmt.Println("\nGitHub Statistics:")
		fmt.Printf("- Issues synchronized (with 'glued' label): %d\n", glued)
		fmt.Printf("- Issues not synchronized (without 'glued' label): %d\n", unglued)
		fmt.Printf("- Total issues: %d\n", glued+unglued)

		fmt.Println("\nTrello Statistics:")
		fmt.Printf("- Total cards in board: %d\n", totalCards)
		fmt.Printf("- Cards created by glue: %d\n", gluedCards)

		fmt.Println("\nSynchronization status: ", getStatusMessage(glued, unglued, gluedCards))

		return nil
	},
}

func getStatusMessage(glued, unglued, gluedCards int) string {
	if unglued == 0 {
		return "All GitHub issues are synchronized with Trello"
	}

	percentage := float64(glued) / float64(glued+unglued) * 100
	return fmt.Sprintf("%.1f%% synchronized (%d/%d issues)", percentage, glued, glued+unglued)
}
