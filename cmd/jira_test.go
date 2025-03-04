package cmd

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

// setupJiraCommandTest creates a command with output capture for testing
func setupJiraCommandTest() (*cobra.Command, *bytes.Buffer) {
	buf := new(bytes.Buffer)
	cmd := rootCmd
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	return cmd, buf
}

// TestParseChildIssuesAlt tests the parseChildIssues function with various inputs
func TestParseChildIssuesAlt(t *testing.T) {
	tests := []struct {
		name        string
		description string
		gitHubDomain string
		expected    []int
	}{
		{
			name:        "empty description",
			description: "",
			gitHubDomain: "github.com",
			expected:    []int{},
		},
		{
			name:        "description with no links",
			description: "This is a description with no links.\n\n## Issues\nNo issues here.",
			gitHubDomain: "github.com",
			expected:    []int{},
		},
		{
			name:        "description with one link",
			description: "Intro text\n\n## Issues\nSee https://github.com/org/repo/issues/123 for more details.",
			gitHubDomain: "github.com",
			expected:    []int{123},
		},
		{
			name:        "description with multiple links",
			description: "Intro text\n\n## Issues\nRelated to https://github.com/org/repo/issues/123 and https://github.com/org/repo/issues/456",
			gitHubDomain: "github.com",
			expected:    []int{123, 456},
		},
		{
			name:        "description with custom domain",
			description: "Intro text\n\n## Issues\nSee https://custom-github.company.com/org/repo/issues/123 for more details.",
			gitHubDomain: "custom-github.company.com",
			expected:    []int{123},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseChildIssues(tt.description, tt.gitHubDomain)
			if len(result) != len(tt.expected) {
				t.Errorf("parseChildIssues() returned %d issues, want %d", len(result), len(tt.expected))
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("parseChildIssues()[%d] = %d, want %d", i, v, tt.expected[i])
				}
			}
		})
	}
}
