package github

import (
	"fmt"
	"log"

	"github.com/dolaszy/glue/internal/github"
	"github.com/spf13/cobra"
)

// InitCmd represents the github init command
var InitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize GitHub repository with required labels",
	Long: `This command initializes a GitHub repository with the necessary labels
for integration with project management tools. It will create the following labels
if they don't already exist:
- 'story'
- 'feature'
- 'glued'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		repository, err := cmd.Flags().GetString("repository")
		if err != nil {
			return err
		}

		if repository == "" {
			return fmt.Errorf("repository flag is required")
		}

		fmt.Printf("Initializing GitHub repository: %s\n", repository)

		// Initialize GitHub client
		client := github.NewClient()

		// Create the required labels
		err = client.InitializeLabels(repository)
		if err != nil {
			log.Fatalf("Failed to initialize labels: %v", err)
			return err
		}

		fmt.Println("Repository successfully initialized with required labels")
		return nil
	},
}
