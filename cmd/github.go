package cmd

import (
	"github.com/dolaszy/glue/cmd/github"
	"github.com/spf13/cobra"
)

// githubCmd represents the GitHub command
var githubCmd = &cobra.Command{
	Use:   "github",
	Short: "GitHub related commands",
	Long:  `Commands for managing GitHub repository integration with project management tools.`,
}

func init() {
	// Add the init subcommand
	githubCmd.AddCommand(github.InitCmd)
}
