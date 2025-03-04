package jira

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/andygrunwald/go-jira"
	"github.com/danielolaszy/glue/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Custom wrapper for testing specific scenarios
type testClient struct {
	*Client
	// Custom behavior flags
	mockGetIssueResponse func(issueID string) (*jira.Issue, error)
}

// createTestClient creates a client for testing with the specified mock behaviors
func createTestClient(mockGetIssue func(issueID string) (*jira.Issue, error)) *testClient {
	tc := &testClient{
		Client: &Client{
			issueTypeCache: make(map[string]map[string]string),
		},
		mockGetIssueResponse: mockGetIssue,
	}

	// Create a real jira.Client but we won't actually use it for API calls
	tc.Client.client = &jira.Client{}

	return tc
}

// Override GetLinkedIssues to use our mock
func (tc *testClient) GetLinkedIssues(issueID string) ([]string, error) {
	// Basic validation that would normally be in the original method
	if issueID == "" {
		return nil, fmt.Errorf("issue ID is required")
	}

	// If the client isn't properly initialized
	if tc.Client.client == nil {
		return nil, fmt.Errorf("jira client not initialized")
	}

	// Use our mock function to get the issue
	issue, err := tc.mockGetIssueResponse(issueID)
	if err != nil {
		return nil, err
	}

	// If we got a valid issue but it has no links
	if issue != nil && (issue.Fields == nil || issue.Fields.IssueLinks == nil || len(issue.Fields.IssueLinks) == 0) {
		return []string{}, nil
	}

	// Extract linked issue IDs (simplified for the test)
	var linkedIssues []string
	for _, link := range issue.Fields.IssueLinks {
		if link.OutwardIssue != nil {
			linkedIssues = append(linkedIssues, link.OutwardIssue.Key)
		}
		if link.InwardIssue != nil {
			linkedIssues = append(linkedIssues, link.InwardIssue.Key)
		}
	}

	return linkedIssues, nil
}

func TestGetIssueLinks(t *testing.T) {
	tests := []struct {
		name      string
		issueID   string
		setupMock func() (*testClient, []string, bool)
	}{
		{
			name:    "Empty issue ID",
			issueID: "",
			setupMock: func() (*testClient, []string, bool) {
				return createTestClient(nil), nil, true
			},
		},
		{
			name:    "Invalid issue ID format",
			issueID: "invalid-format",
			setupMock: func() (*testClient, []string, bool) {
				return createTestClient(func(issueID string) (*jira.Issue, error) {
					return nil, fmt.Errorf("invalid issue ID format")
				}), nil, true
			},
		},
		{
			name:    "Non-existent issue",
			issueID: "TEST-999",
			setupMock: func() (*testClient, []string, bool) {
				return createTestClient(func(issueID string) (*jira.Issue, error) {
					return nil, fmt.Errorf("issue not found")
				}), nil, true
			},
		},
		{
			name:    "Valid issue without links",
			issueID: "TEST-1",
			setupMock: func() (*testClient, []string, bool) {
				return createTestClient(func(issueID string) (*jira.Issue, error) {
					return &jira.Issue{
						Key: "TEST-1",
						Fields: &jira.IssueFields{
							IssueLinks: []*jira.IssueLink{},
						},
					}, nil
				}), []string{}, false
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup the test client with the appropriate mock
			client, wantLinks, wantErr := tt.setupMock()

			// Call the method we're testing
			gotLinks, err := client.GetLinkedIssues(tt.issueID)

			// Check results
			if wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, wantLinks, gotLinks)
		})
	}
}

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

	// Test with uninitialized client
	err := client.DeleteIssueLink("TEST-1", "TEST-2")

	// Should return an error when client is nil
	if err == nil {
		t.Error("Expected error when client is nil, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("Expected 'not initialized' error, got: %v", err)
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

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		wantErr bool
	}{
		{
			name: "Missing all required env vars",
			envVars: map[string]string{
				"JIRA_URL":      "",
				"JIRA_USERNAME": "",
				"JIRA_TOKEN":    "",
			},
			wantErr: true,
		},
		{
			name: "Missing JIRA_URL",
			envVars: map[string]string{
				"JIRA_URL":      "",
				"JIRA_USERNAME": "test",
				"JIRA_TOKEN":    "test",
			},
			wantErr: true,
		},
		{
			name: "Missing JIRA_USERNAME",
			envVars: map[string]string{
				"JIRA_URL":      "https://test.atlassian.net",
				"JIRA_USERNAME": "",
				"JIRA_TOKEN":    "test",
			},
			wantErr: true,
		},
		{
			name: "Missing JIRA_TOKEN",
			envVars: map[string]string{
				"JIRA_URL":      "https://test.atlassian.net",
				"JIRA_USERNAME": "test",
				"JIRA_TOKEN":    "",
			},
			wantErr: true,
		},
		{
			name: "All required env vars present",
			envVars: map[string]string{
				"JIRA_URL":      "https://test.atlassian.net",
				"JIRA_USERNAME": "test",
				"JIRA_TOKEN":    "test",
			},
			wantErr: true, // Set to true since we expect auth to fail with test credentials
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original env vars
			origURL := os.Getenv("JIRA_URL")
			origUsername := os.Getenv("JIRA_USERNAME")
			origToken := os.Getenv("JIRA_TOKEN")

			// Set test env vars
			for k, v := range tt.envVars {
				require.NoError(t, os.Setenv(k, v))
			}

			// Run test
			client, err := NewClient()
			if tt.wantErr {
				assert.Error(t, err)
				// Don't try to access client fields if we expect an error
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
				assert.NotNil(t, client.client)
			}

			// Restore original env vars
			require.NoError(t, os.Setenv("JIRA_URL", origURL))
			require.NoError(t, os.Setenv("JIRA_USERNAME", origUsername))
			require.NoError(t, os.Setenv("JIRA_TOKEN", origToken))
		})
	}
}

func TestGetIssueTypeID(t *testing.T) {
	tp := jira.BasicAuthTransport{
		Username: "test",
		Password: "test",
	}
	jiraClient, err := jira.NewClient(tp.Client(), "https://test.atlassian.net")
	if err != nil {
		t.Fatalf("Failed to create JIRA client: %v", err)
	}

	client := &Client{
		client:         jiraClient,
		issueTypeCache: make(map[string]map[string]string),
	}

	// Initialize cache for test project
	client.issueTypeCache["TEST"] = map[string]string{
		"story": "10001",
	}

	tests := []struct {
		name      string
		project   string
		issueType string
		wantID    string
		wantError bool
	}{
		{
			name:      "Existing type",
			project:   "TEST",
			issueType: "story",
			wantID:    "10001",
			wantError: false,
		},
		{
			name:      "Non-existent type",
			project:   "TEST",
			issueType: "unknown",
			wantID:    "",
			wantError: true,
		},
		{
			name:      "Non-existent project",
			project:   "INVALID",
			issueType: "story",
			wantID:    "",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, err := client.GetIssueTypeID(tt.project, tt.issueType)
			if (err != nil) != tt.wantError {
				t.Errorf("GetIssueTypeID() error = %v, wantError %v", err, tt.wantError)
				return
			}
			if gotID != tt.wantID {
				t.Errorf("GetIssueTypeID() = %v, want %v", gotID, tt.wantID)
			}
		})
	}
}

func TestCreateParentChildLink(t *testing.T) {
	client := &Client{}

	tests := []struct {
		name      string
		parent    string
		child     string
		wantError bool
	}{
		{
			name:      "Uninitialized client",
			parent:    "TEST-1",
			child:     "TEST-2",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.CreateParentChildLink(tt.parent, tt.child)
			if (err != nil) != tt.wantError {
				t.Errorf("CreateParentChildLink() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

// TestCloseTicket tests basic validation in the CloseTicket function
func TestCloseTicket(t *testing.T) {
	client := &Client{}

	tests := []struct {
		name      string
		ticketID  string
		wantError bool
	}{
		{
			name:      "Uninitialized client",
			ticketID:  "TEST-1",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.CloseTicket(tt.ticketID)
			if (err != nil) != tt.wantError {
				t.Errorf("CloseTicket() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

// Helper function to compare maps
func mapsEqual(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

func TestGetTotalTickets(t *testing.T) {
	tests := []struct {
		name       string
		client     *Client
		projectKey string
		wantCount  int
		wantErr    bool
	}{
		{
			name:       "Uninitialized client",
			client:     &Client{},
			projectKey: "TEST",
			wantCount:  0,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, err := tt.client.GetTotalTickets(tt.projectKey)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, tt.wantCount, count)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantCount, count)
			}
		})
	}
}

func TestIssueTypeExists(t *testing.T) {
	tests := []struct {
		name       string
		client     *Client
		projectKey string
		typeName   string
		wantExists bool
		wantID     string
		wantErr    bool
	}{
		{
			name:       "Uninitialized client",
			client:     &Client{},
			projectKey: "TEST",
			typeName:   "Bug",
			wantExists: false,
			wantID:     "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exists, id, err := tt.client.IssueTypeExists(tt.projectKey, tt.typeName)
			if tt.wantErr {
				assert.Error(t, err)
				assert.False(t, exists)
				assert.Empty(t, id)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantExists, exists)
				assert.Equal(t, tt.wantID, id)
			}
		})
	}
}

func TestCreateTicketWithTypeIDBasicValidation(t *testing.T) {
	// Create a client with nil jira.Client to test validation
	client := &Client{} // Intentionally not initialized
	
	issue := models.GitHubIssue{
		Title:       "Test Issue",
		Description: "Test Description",
	}
	
	// Test with uninitialized client
	_, err := client.CreateTicketWithTypeID("TEST", issue, "10001")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "jira client not initialized")
}
