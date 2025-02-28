// Package main is the entry point for the Glue CLI application.
package main

import (
	"fmt"
	"os"

	"github.com/danielolaszy/glue/cmd"
)

// main is the entry point of the application.
// It executes the root command and handles any errors that occur.
func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
