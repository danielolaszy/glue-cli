package cmd

import (
	"github.com/dolaszy/glue/cmd/jira"
	"github.com/spf13/cobra"
)

// jiraCmd represents the JIRA command
var jiraCmd = &cobra.Command{
	Use:   "jira",
	Short: "JIRA related commands",
	Long:  `Commands for synchronizing GitHub issues with JIRA boards.`,
}

func init() {
	// Add the sync subcommand
	jiraCmd.AddCommand(jira.SyncCmd)

	// Add the status subcommand
	jiraCmd.AddCommand(jira.StatusCmd)
}
