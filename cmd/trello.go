package cmd

import (
	"github.com/dolaszy/glue/cmd/trello"
	"github.com/spf13/cobra"
)

// trelloCmd represents the Trello command
var trelloCmd = &cobra.Command{
	Use:   "trello",
	Short: "Trello related commands",
	Long:  `Commands for synchronizing GitHub issues with Trello boards.`,
}

func init() {
	// Add the sync subcommand
	trelloCmd.AddCommand(trello.SyncCmd)

	// Add the status subcommand
	trelloCmd.AddCommand(trello.StatusCmd)
}
