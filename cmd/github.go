package cmd

import (
	"fmt"
	"github.com/danielolaszy/glue/internal/github"
	"github.com/danielolaszy/glue/internal/logging"
	"regexp"

	"github.com/spf13/cobra"
)

var githubCmd = &cobra.Command{
	Use:   "github",
	Short: "Initialize GitHub repository",
	Long: `Initialize a GitHub repository with the necessary labels for integration.
	
This command creates the required labels in your GitHub repository:
- 'type: feature' - For feature requests
- 'type: story' - For user stories`,
	RunE: func(cmd *cobra.Command, args []string) error {
		repository, err := cmd.Flags().GetString("repository")
		if err != nil {
			return err
		}

		githubClient, err := github.NewClient()
		if err != nil {
			return fmt.Errorf("failed to initialize GitHub client: %w", err)
		}

		//err = githubClient.AddLabels(repository, 1, "jira-id: LEGCR-123")
		//if err != nil {
		//	return err
		//}

		// Create a pattern to match JIRA ID labels and extract the project key
		jiraPattern := regexp.MustCompile(`^jira-id: ([A-Z]+)-\d+$`)
		matching, err := githubClient.HasLabelMatching(repository, 1, jiraPattern)
		if err != nil {
			return err
		}
		fmt.Println(matching)

		has, err := githubClient.HasLabel(repository, 2, "type: feature")
		if err != nil {
			return err
		}
		if has == true {
			logging.Info("gitHub issue is a feature")
		} else {
			logging.Info("gitHub issue is not a feature")
		}

		return nil
	},
}
