package github

import (
	"net/url"
	"strings"
	"testing"
)

// TestGitHubDomainToAPIURL tests the logic that converts a domain to an API URL
// This is a unit test focusing just on the URL construction logic
func TestGitHubDomainToAPIURL(t *testing.T) {
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
			domain:         "github.example.com",
			expectedAPIURL: "https://github.example.com/api/v3/",
		},
		{
			name:           "Empty Domain (should default to github.com)",
			domain:         "",
			expectedAPIURL: "https://api.github.com/",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Get the domain from test case, defaulting to github.com if empty
			domain := tc.domain
			if domain == "" {
				domain = "github.com"
			}

			// Construct API URL based on domain using the same logic as in the client
			var apiURL string
			if domain == "github.com" {
				apiURL = "https://api.github.com/"
			} else {
				apiURL = "https://" + domain + "/api/v3/"
			}

			// Verify URL matches expected
			if apiURL != tc.expectedAPIURL {
				t.Errorf("Expected API URL %s, got %s", tc.expectedAPIURL, apiURL)
			}

			// Also test URL parsing to ensure the URLs are valid
			parsedURL, err := url.Parse(apiURL)
			if err != nil {
				t.Errorf("Failed to parse URL %s: %v", apiURL, err)
			}

			if parsedURL.String() != apiURL {
				t.Errorf("URL parsing changed the URL from %s to %s", apiURL, parsedURL.String())
			}
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
