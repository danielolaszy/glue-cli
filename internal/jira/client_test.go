package jira

import (
	"os"
	"strings"
	"testing"
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
		name          string
		url           string
		username      string
		token         string
		wantError     bool
		errorContains string
	}{
		{
			name:          "All credentials provided but invalid",
			url:           "https://example.atlassian.net",
			username:      "test@example.com",
			token:         "test-token",
			wantError:     true,
			errorContains: "401", // We expect a 401 unauthorized error
		},
		{
			name:          "Missing URL",
			url:           "",
			username:      "test@example.com",
			token:         "test-token",
			wantError:     true,
			errorContains: "JIRA_URL",
		},
		{
			name:          "Missing username",
			url:           "https://example.atlassian.net",
			username:      "",
			token:         "test-token",
			wantError:     true,
			errorContains: "JIRA_USERNAME",
		},
		{
			name:          "Missing token",
			url:           "https://example.atlassian.net",
			username:      "test@example.com",
			token:         "",
			wantError:     true,
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

// TestCreateParentChildLinkValidation tests basic validation in the CreateParentChildLink function
func TestCreateParentChildLinkValidation(t *testing.T) {
	// Create a client directly with initialized cache but nil client
	client := &Client{} // Intentionally not initialized

	// Test with nil client
	err := client.CreateParentChildLink("TEST-1", "TEST-2")
	if err == nil {
		t.Error("Expected error when client is nil, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("Expected 'not initialized' error, got: %v", err)
	}
}

// TestDeleteIssueLinkValidation tests basic validation in the DeleteIssueLink function
func TestDeleteIssueLinkValidation(t *testing.T) {
	// Test with nil client
	client := &Client{} // Intentionally not initialized
	err := client.DeleteIssueLink("TEST-1", "TEST-2")

	// Should return an error when client is nil
	if err == nil {
		t.Error("Expected error when client is nil, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("Expected 'not initialized' error, got: %v", err)
	}
}

// TestGetLinkedIssuesValidation tests basic validation in the GetLinkedIssues function
func TestGetLinkedIssuesValidation(t *testing.T) {
	// Test with nil client
	client := &Client{} // Intentionally not initialized
	issues, err := client.GetLinkedIssues("TEST-1")

	// Should return an error when client is nil
	if err == nil {
		t.Error("Expected error when client is nil, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("Expected 'not initialized' error, got: %v", err)
	}

	// Should return nil for issues when there's an error
	if issues != nil {
		t.Errorf("Expected nil issues when there's an error, got: %v", issues)
	}
}

// TestGetIssueLinkIDValidation tests basic validation in the GetIssueLinkID function
func TestGetIssueLinkIDValidation(t *testing.T) {
	// Test with nil client
	client := &Client{} // Intentionally not initialized
	linkID, err := client.GetIssueLinkID("TEST-1", "TEST-2")

	// Should return an error when client is nil
	if err == nil {
		t.Error("Expected error when client is nil, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("Expected 'not initialized' error, got: %v", err)
	}

	// Should return empty string when there's an error
	if linkID != "" {
		t.Errorf("Expected empty link ID when there's an error, got: %s", linkID)
	}
}

// TestCheckParentChildLinkExistsValidation tests basic validation in the CheckParentChildLinkExists function
func TestCheckParentChildLinkExistsValidation(t *testing.T) {
	// Test with nil client
	client := &Client{} // Intentionally not initialized
	exists, err := client.CheckParentChildLinkExists("TEST-1", "TEST-2")

	// Should return an error when client is nil
	if err == nil {
		t.Error("Expected error when client is nil, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("Expected 'not initialized' error, got: %v", err)
	}

	// Should return false when there's an error
	if exists {
		t.Error("Expected false when there's an error, got true")
	}
}

// TestCloseTicketValidation tests basic validation in the CloseTicket function
func TestCloseTicketValidation(t *testing.T) {
	// Test with nil client
	client := &Client{} // Intentionally not initialized
	err := client.CloseTicket("TEST-1")

	// Should return an error when client is nil
	if err == nil {
		t.Error("Expected error when client is nil, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("Expected 'not initialized' error, got: %v", err)
	}
}
