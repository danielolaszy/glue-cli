// Package cmd provides the command-line interface for the Glue CLI tool.
package cmd

import (
	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands.
// It serves as the entry point for the Glue CLI application.
var rootCmd = &cobra.Command{
	Use:   "glue",
	Short: "Glue synchronizes GitHub issues with project management tools",
	Long: `Glue is a CLI tool that synchronizes GitHub issues with project management tools
like JIRA. It enables seamless integration between your GitHub repository
and your preferred project management platform.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once.
func Execute() error {
	return rootCmd.Execute()
}

// init is called when the package is initialized. It sets up the command structure
// and defines flags that are shared across all commands.
func init() {
	// Add persistent flags that will be available to all commands
	rootCmd.PersistentFlags().StringP("repository", "r", "", "GitHub repository name (e.g., 'username/repo')")

	// Add the JIRA command
	rootCmd.AddCommand(jiraCmd)
}
