package jira

import (
	"os"
	"strings"
	"testing"

	"github.com/danielolaszy/glue/pkg/models"
)

func TestJiraClientCredentialValidation(t *testing.T) {
	// Save original env vars to restore later
	origURL := os.Getenv("JIRA_URL")
	origUsername := os.Getenv("JIRA_USERNAME")
	origToken := os.Getenv("JIRA_TOKEN")

	// Cleanup after test
	defer func() {
		os.Setenv("JIRA_URL", origURL)
		os.Setenv("JIRA_USERNAME", origUsername)
		os.Setenv("JIRA_TOKEN", origToken)
	}()

	testCases := []struct {
		name      string
		url       string
		username  string
		token     string
		wantError bool
		errorContains string
	}{
		{
			name:      "All credentials provided but invalid",
			url:       "https://example.atlassian.net",
			username:  "test@example.com",
			token:     "test-token",
			wantError: true,
			errorContains: "401", // We expect a 401 unauthorized error
		},
		{
			name:      "Missing URL",
			url:       "",
			username:  "test@example.com",
			token:     "test-token",
			wantError: true,
			errorContains: "JIRA_URL",
		},
		{
			name:      "Missing username",
			url:       "https://example.atlassian.net",
			username:  "",
			token:     "test-token",
			wantError: true,
			errorContains: "JIRA_USERNAME",
		},
		{
			name:      "Missing token",
			url:       "https://example.atlassian.net",
			username:  "test@example.com",
			token:     "",
			wantError: true,
			errorContains: "JIRA_TOKEN",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set test env vars
			os.Setenv("JIRA_URL", tc.url)
			os.Setenv("JIRA_USERNAME", tc.username)
			os.Setenv("JIRA_TOKEN", tc.token)

			// Attempt to create client
			_, err := NewClient()

			// Check error
			if (err != nil) != tc.wantError {
				t.Errorf("Expected error: %v, got error: %v", tc.wantError, err != nil)
			}

			// If we expected an error, make sure it contains expected text
			if tc.wantError && err != nil {
				if !contains(err.Error(), tc.errorContains) {
					t.Errorf("Error should contain '%s': %v", tc.errorContains, err)
				}
			}
		})
	}
}

// TestIssueTypeCache tests that the client's issue type cache is properly initialized and used
func TestIssueTypeCache(t *testing.T) {
	// Create a client directly, bypassing NewClient to avoid API calls
	client := &Client{
		issueTypeCache: make(map[string]map[string]string),
	}

	// The cache should be empty
	if len(client.issueTypeCache) != 0 {
		t.Errorf("Expected empty cache, got %d entries", len(client.issueTypeCache))
	}

	// Test adding to the cache
	projectKey := "TEST"
	typeName := "Story"
	typeID := "10001"

	// Initialize project in cache if not exists
	if _, exists := client.issueTypeCache[projectKey]; !exists {
		client.issueTypeCache[projectKey] = make(map[string]string)
	}

	// Add type to cache
	client.issueTypeCache[projectKey][typeName] = typeID

	// Verify cache contains the entry
	if client.issueTypeCache[projectKey][typeName] != typeID {
		t.Errorf("Expected cache to contain %s:%s=%s, got %s", 
			projectKey, typeName, typeID, client.issueTypeCache[projectKey][typeName])
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// MockClient is a simplified test double for the JIRA client
type MockClient struct{}

// TestCreateTicketWithMock tests the CreateTicket function with mocked dependencies
func TestCreateTicketBasics(t *testing.T) {
	// Create a test GitHub issue
	issue := models.GitHubIssue{
		Number:      123,
		Title:       "Test Issue",
		Description: "This is a test issue description",
		Labels:      []string{"bug", "priority:high"},
	}

	// Create a client directly with initialized cache
	client := &Client{
		issueTypeCache: make(map[string]map[string]string),
	}

	// Test validation when client is nil
	client.client = nil
	_, err := client.CreateTicket("TEST", issue, "Story")
	if err == nil {
		t.Error("Expected error when client is nil, got nil")
	}
	if !contains(err.Error(), "not initialized") {
		t.Errorf("Expected error to mention 'not initialized', got: %v", err)
	}
}

// TestCreateParentChildLinkValidation tests basic validation in the CreateParentChildLink function
func TestCreateParentChildLinkValidation(t *testing.T) {
	// Create a client directly with initialized cache but nil client
	client := &Client{
		issueTypeCache: make(map[string]map[string]string),
	}
	
	// Test validation when client is nil
	client.client = nil
	err := client.CreateParentChildLink("TEST-1", "TEST-2")
	if err == nil {
		t.Error("Expected error when client is nil, got nil")
	}
	if !contains(err.Error(), "not initialized") {
		t.Errorf("Expected error to mention 'not initialized', got: %v", err)
	}
} 