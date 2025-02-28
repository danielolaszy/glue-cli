// Package main is the entry point for the Glue CLI application.
package main

import (
	"fmt"
	"os"

	"github.com/danielolaszy/glue/cmd"
	"github.com/danielolaszy/glue/internal/logging"
)

// main is the entry point of the application.
// It executes the root command and handles any errors that occur.
func main() {
	// Initialize logging (already done in init, but we can customize here if needed)
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}
	
	logging.Info("starting glue cli", "version", "1.0.0", "log_level", logLevel)
	
	if err := cmd.Execute(); err != nil {
		logging.Error("command execution failed", "error", err)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
