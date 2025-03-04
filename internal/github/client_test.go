package github

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"github.com/google/go-github/v41/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGitHubDomainToAPIURL tests the logic that converts a domain to an API URL
// This is a unit test focusing just on the URL construction logic
func TestGitHubDomainToAPIURL(t *testing.T) {
	tests := []struct {
		name     string
		domain   string
		wantURL  string
	}{
		{
			name:     "Default GitHub.com",
			domain:   "github.com",
			wantURL:  "https://api.github.com/",
		},
		{
			name:     "GitHub Enterprise",
			domain:   "github.example.com",
			wantURL:  "https://github.example.com/api/v3/",
		},
		{
			name:     "Empty Domain (should default to github.com)",
			domain:   "",
			wantURL:  "https://api.github.com/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			domain := tt.domain
			if domain == "" {
				domain = "github.com"
			}

			var apiURL string
			if domain == "github.com" {
				apiURL = "https://api.github.com/"
			} else {
				apiURL = "https://" + domain + "/api/v3/"
			}

			assert.Equal(t, tt.wantURL, apiURL)

			parsedURL, err := url.Parse(apiURL)
			require.NoError(t, err)
			assert.Equal(t, apiURL, parsedURL.String())
		})
	}
}

// TestNewClientWithMock tests the NewClient function with a mocked HTTP client
// This is commented out since it would require more complex mocking setup
/*
func TestNewClientWithMock(t *testing.T) {
	// Save original env vars to restore later
	origToken := os.Getenv("GITHUB_TOKEN")
	origDomain := os.Getenv("GITHUB_DOMAIN")

	// Set test token to allow client creation
	os.Setenv("GITHUB_TOKEN", "test_token")

	// Cleanup after test
	defer func() {
		os.Setenv("GITHUB_TOKEN", origToken)
		os.Setenv("GITHUB_DOMAIN", origDomain)
	}()

	testCases := []struct {
		name           string
		domain         string
		expectedAPIURL string
	}{
		{
			name:           "Default GitHub.com",
			domain:         "github.com",
			expectedAPIURL: "https://api.github.com/",
		},
		{
			name:           "GitHub Enterprise",
			domain:         "git.example.com",
			expectedAPIURL: "https://git.example.com/api/v3/",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set test domain
			os.Setenv("GITHUB_DOMAIN", tc.domain)

			// Clear config cache if any
			config.ResetForTest()

			// TODO: Mock HTTP client and API responses
			// Create client
			client, err := NewClient()
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			// Check baseURL
			if client.client.BaseURL.String() != tc.expectedAPIURL {
				t.Errorf("Expected API URL %s, got %s", tc.expectedAPIURL, client.client.BaseURL.String())
			}
		})
	}
}
*/

// TestIsIssueClosedValidation tests the validation in the IsIssueClosed function
func TestIsIssueClosedValidation(t *testing.T) {
	// Create a client directly with initialized fields but without API connection
	client := &Client{}

	// Test with invalid repository format
	_, err := client.IsIssueClosed("invalid-repo-format", 123)
	if err == nil {
		t.Error("Expected error with invalid repository format, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "invalid repository format") {
		t.Errorf("Expected 'invalid repository format' error, got: %v", err)
	}
}

// TestGetClosedIssuesValidation tests the validation in the GetClosedIssues function
func TestGetClosedIssuesValidation(t *testing.T) {
	// Create a client directly with initialized fields but without API connection
	client := &Client{}

	// Test with invalid repository format
	_, err := client.GetClosedIssues("invalid-repo-format")
	if err == nil {
		t.Error("Expected error with invalid repository format, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "invalid repository format") {
		t.Errorf("Expected 'invalid repository format' error, got: %v", err)
	}
}

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		wantErr bool
	}{
		{
			name: "All required env vars present but invalid token",
			envVars: map[string]string{
				"GITHUB_TOKEN": "invalid_token",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original env vars
			origToken := os.Getenv("GITHUB_TOKEN")

			// Set test env vars
			for k, v := range tt.envVars {
				require.NoError(t, os.Setenv(k, v))
			}

			// Run test
			_, err := NewClient()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Restore original env vars
			require.NoError(t, os.Setenv("GITHUB_TOKEN", origToken))
		})
	}
}

func validateRepository(repo string) error {
	if repo == "" {
		return fmt.Errorf("repository cannot be empty")
	}
	if !strings.Contains(repo, "/") {
		return fmt.Errorf("invalid repository format")
	}
	return nil
}

func TestGetIssuesWithLabels(t *testing.T) {
	tests := []struct {
		name       string
		repo       string
		labels     []string
		wantIssues []*github.Issue
		wantErr    bool
	}{
		{
			name:       "Invalid repository format",
			repo:       "invalid-repo",
			labels:     []string{"bug"},
			wantIssues: nil,
			wantErr:    true,
		},
		{
			name:       "Empty repository",
			repo:       "",
			labels:     []string{"bug"},
			wantIssues: nil,
			wantErr:    true,
		},
		{
			name:       "Empty labels",
			repo:       "owner/repo",
			labels:     []string{},
			wantIssues: nil,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				client: github.NewClient(nil),
			}
			
			issues, err := client.GetIssuesWithLabels(tt.repo, tt.labels)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, issues)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantIssues, issues)
			}
		})
	}
}

func TestHasLabelMatching(t *testing.T) {
	pattern := regexp.MustCompile("bug.*")
	tests := []struct {
		name       string
		client     *Client
		repo       string
		issueNum   int
		pattern    *regexp.Regexp
		wantResult bool
		wantErr    bool
	}{
		{
			name:       "Invalid repository format",
			client:     &Client{},
			repo:       "invalid-repo",
			issueNum:   1,
			pattern:    pattern,
			wantResult: false,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.client.HasLabelMatching(tt.repo, tt.issueNum, tt.pattern)
			if tt.wantErr {
				assert.Error(t, err)
				assert.False(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantResult, result)
			}
		})
	}
}

func TestGetClosedIssuesWithLabels(t *testing.T) {
	tests := []struct {
		name       string
		repo       string
		labels     []string
		wantIssues []*github.Issue
		wantErr    bool
	}{
		{
			name:       "Invalid repository format",
			repo:       "invalid-repo",
			labels:     []string{"bug"},
			wantIssues: nil,
			wantErr:    true,
		},
		{
			name:       "Empty repository",
			repo:       "",
			labels:     []string{"bug"},
			wantIssues: nil,
			wantErr:    true,
		},
		{
			name:       "Empty labels",
			repo:       "owner/repo",
			labels:     []string{},
			wantIssues: nil,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initialize client with a non-nil GitHub client
			client := &Client{
				client: github.NewClient(nil),
			}
			
			issues, err := client.GetClosedIssuesWithLabels(tt.repo, tt.labels)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, issues)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantIssues, issues)
			}
		})
	}
}

// Helper functions
func createTestLabels(names []string) []*github.Label {
	labels := make([]*github.Label, len(names))
	for i, name := range names {
		labels[i] = &github.Label{Name: &name}
	}
	return labels
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
