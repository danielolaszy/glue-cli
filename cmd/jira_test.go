package cmd

import (
	"testing"
)

func TestParseChildIssues(t *testing.T) {
	testCases := []struct {
		name        string
		description string
		domain      string
		expected    []string
	}{
		{
			name: "Using non-enterprise GitHub",
			description: `
				## Description
				This feature provides user authentication functionality.
				
				## Issues
				- https://github.com/org/repo/issues/1
				- https://github.com/org/repo/issues/2
				- https://github.com/org/repo/issues/3`,
			domain: "github.com",
			expected: []string{
				"https://github.com/org/repo/issues/1",
				"https://github.com/org/repo/issues/2",
				"https://github.com/org/repo/issues/3",
			},
		},
		{
			name: "Using enterprise GitHub domain",
			description: `
				## Description
				This is a GitHub Feature

				## Issues
				- https://github.example.com/org/repo/issues/1
				- https://github.example.com/org/repo/issues/2
				`,
			domain: "github.example.com",
			expected: []string{
				"https://github.example.com/org/repo/issues/1",
				"https://github.example.com/org/repo/issues/2",
			},
		},
		{
			name: "Mixed domains should only match configured domain",
			description: `
				## Issues
				- https://github.com/org/repo/issues/1
				- https://github.example.com/org/repo/issues/2
				`,
			domain: "github.example.com",
			expected: []string{
				"https://github.example.com/org/repo/issues/2",
			},
		},
		{
			name: "No issues section",
			description: `
				## Description
				This is a GitHub Feature with no issues linked to it.
				`,
			domain:   "github.example.com",
			expected: []string{},
		},
		{
			name: "Empty issues section",
			description: `
				## Description
				This is a GitHub Feature with an empty issues section.

				## Issues
				`,
			domain:   "github.example.com",
			expected: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := parseChildIssues(tc.description, tc.domain)

			// Check array length
			if len(result) != len(tc.expected) {
				t.Fatalf("Expected %d links, got %d: %v", len(tc.expected), len(result), result)
			}

			// Check each value
			for i := range tc.expected {
				if i >= len(result) {
					t.Fatalf("Missing expected link: %s", tc.expected[i])
				}

				if result[i] != tc.expected[i] {
					t.Errorf("Expected link %s, got %s", tc.expected[i], result[i])
				}
			}
		})
	}
}

func TestParseGitHubIssueLink(t *testing.T) {
	testCases := []struct {
		name         string
		link         string
		domain       string
		wantRepo     string
		wantIssueNum int
		wantErr      bool
	}{
		{
			name:         "Standard GitHub issue",
			link:         "https://github.com/org/repo/issues/123",
			domain:       "github.com",
			wantRepo:     "org/repo",
			wantIssueNum: 123,
			wantErr:      false,
		},
		{
			name:         "Enterprise GitHub issue",
			link:         "https://git.example.com/org/repo/issues/456",
			domain:       "git.example.com",
			wantRepo:     "org/repo",
			wantIssueNum: 456,
			wantErr:      false,
		},
		{
			name:         "Invalid issue number",
			link:         "https://github.com/org/repo/issues/abc",
			domain:       "github.com",
			wantRepo:     "",
			wantIssueNum: 0,
			wantErr:      true,
		},
		{
			name:         "Domain mismatch",
			link:         "https://github.com/org/repo/issues/123",
			domain:       "git.example.com",
			wantRepo:     "",
			wantIssueNum: 0,
			wantErr:      true,
		},
		{
			name:         "Invalid URL format",
			link:         "https://github.com/org/issues/123",
			domain:       "github.com",
			wantRepo:     "",
			wantIssueNum: 0,
			wantErr:      true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			repo, issueNum, err := parseGitHubIssueLink(tc.link, tc.domain)

			// Check error
			if (err != nil) != tc.wantErr {
				t.Fatalf("Expected error: %v, got error: %v", tc.wantErr, err)
			}

			// If we expected an error, don't check the return values
			if tc.wantErr {
				return
			}

			// Check repo
			if repo != tc.wantRepo {
				t.Errorf("Expected repo %s, got %s", tc.wantRepo, repo)
			}

			// Check issue number
			if issueNum != tc.wantIssueNum {
				t.Errorf("Expected issue number %d, got %d", tc.wantIssueNum, issueNum)
			}
		})
	}
}
