package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "glue",
	Short: "Glue synchronizes GitHub issues with project management tools",
	Long: `Glue is a CLI tool that synchronizes GitHub issues with project management tools
like JIRA and Trello. It enables seamless integration between your GitHub repository
and your preferred project management platform.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Add persistent flags that will be available to all commands
	rootCmd.PersistentFlags().StringP("repository", "r", "", "GitHub repository name (e.g., 'username/repo')")
	rootCmd.PersistentFlags().StringP("board", "b", "", "Project management board name")

	// Add the GitHub command
	rootCmd.AddCommand(githubCmd)

	// Add the JIRA command
	rootCmd.AddCommand(jiraCmd)

	// Add the Trello command
	rootCmd.AddCommand(trelloCmd)
}
