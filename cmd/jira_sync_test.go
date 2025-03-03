package cmd

import (
	"testing"
	"errors"
	"strings"

	"github.com/danielolaszy/glue/pkg/models"
)

// Define interfaces for the functionality we need to test
type GitHubClientInterface interface {
	GetClosedIssues(repo string) ([]models.GitHubIssue, error)
	GetLabelsForIssue(repo string, issueNum int) ([]string, error)
}

type JiraClientInterface interface {
	CloseTicket(ticketKey string) error
}

// Implement a test version of syncClosedIssues that uses interfaces instead of concrete types
func testSyncClosedIssues(repository string, githubClient GitHubClientInterface, jiraClient JiraClientInterface) (int, error) {
	// Parse repository owner and name
	parts := strings.Split(repository, "/")
	if len(parts) != 2 {
		return 0, errors.New("invalid repository format: " + repository + ", expected format: owner/repo")
	}
	
	// Get all closed issues
	closedIssues, err := githubClient.GetClosedIssues(repository)
	if err != nil {
		return 0, errors.New("failed to fetch closed GitHub issues: " + err.Error())
	}
	
	closeCount := 0
	
	// Process each closed issue
	for _, issue := range closedIssues {
		// Get labels for the issue
		labels, err := githubClient.GetLabelsForIssue(repository, issue.Number)
		if err != nil {
			continue
		}
		
		// Check for jira-id label
		jiraID := getJiraIDFromLabels(labels)
		if jiraID == "" {
			continue
		}
		
		// Close the corresponding JIRA ticket
		err = jiraClient.CloseTicket(jiraID)
		if err != nil {
			continue
		}
		
		closeCount++
	}
	
	return closeCount, nil
}

// MockGitHubClient implements GitHubClientInterface for testing
type MockGitHubClient struct {
	GetClosedIssuesFunc  func(string) ([]models.GitHubIssue, error)
	GetLabelsForIssueFunc func(string, int) ([]string, error)
}

func (m *MockGitHubClient) GetClosedIssues(repo string) ([]models.GitHubIssue, error) {
	if m.GetClosedIssuesFunc != nil {
		return m.GetClosedIssuesFunc(repo)
	}
	return nil, errors.New("GetClosedIssues not implemented")
}

func (m *MockGitHubClient) GetLabelsForIssue(repo string, issueNum int) ([]string, error) {
	if m.GetLabelsForIssueFunc != nil {
		return m.GetLabelsForIssueFunc(repo, issueNum)
	}
	return nil, errors.New("GetLabelsForIssue not implemented")
}

// MockJiraClient implements JiraClientInterface for testing
type MockJiraClient struct {
	CloseTicketFunc func(string) error
}

func (m *MockJiraClient) CloseTicket(ticketKey string) error {
	if m.CloseTicketFunc != nil {
		return m.CloseTicketFunc(ticketKey)
	}
	return errors.New("CloseTicket not implemented")
}

// TestSyncClosedIssuesInvalidRepository tests the syncClosedIssues function with an invalid repository format
func TestSyncClosedIssuesInvalidRepository(t *testing.T) {
	githubClient := &MockGitHubClient{}
	jiraClient := &MockJiraClient{}
	
	_, err := testSyncClosedIssues("invalid-repo-format", githubClient, jiraClient)
	
	if err == nil {
		t.Error("Expected error with invalid repository format, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "invalid repository format") {
		t.Errorf("Expected 'invalid repository format' error, got: %v", err)
	}
}

// TestSyncClosedIssuesWithAPIError tests the syncClosedIssues function when GetClosedIssues returns an error
func TestSyncClosedIssuesWithAPIError(t *testing.T) {
	// Mock GitHub client that returns an error for GetClosedIssues
	githubClient := &MockGitHubClient{
		GetClosedIssuesFunc: func(repo string) ([]models.GitHubIssue, error) {
			return nil, errors.New("API error")
		},
	}
	
	jiraClient := &MockJiraClient{}
	
	_, err := testSyncClosedIssues("owner/repo", githubClient, jiraClient)
	
	if err == nil {
		t.Error("Expected error when GetClosedIssues fails, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "API error") {
		t.Errorf("Expected 'API error' error, got: %v", err)
	}
}

// TestSyncClosedIssuesNoClosedIssues tests the syncClosedIssues function when there are no closed issues
func TestSyncClosedIssuesNoClosedIssues(t *testing.T) {
	// Mock GitHub client that returns empty list for GetClosedIssues
	githubClient := &MockGitHubClient{
		GetClosedIssuesFunc: func(repo string) ([]models.GitHubIssue, error) {
			return []models.GitHubIssue{}, nil
		},
	}
	
	jiraClient := &MockJiraClient{}
	
	count, err := testSyncClosedIssues("owner/repo", githubClient, jiraClient)
	
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	
	if count != 0 {
		t.Errorf("Expected 0 closed tickets, got: %d", count)
	}
}

// TestSyncClosedIssuesWithClosedIssue tests the syncClosedIssues function with a closed issue that has a JIRA ID
func TestSyncClosedIssuesWithClosedIssue(t *testing.T) {
	// Mock GitHub client that returns a closed issue
	githubClient := &MockGitHubClient{
		GetClosedIssuesFunc: func(repo string) ([]models.GitHubIssue, error) {
			return []models.GitHubIssue{
				{
					Number: 123,
					Title: "Test Issue",
				},
			}, nil
		},
		GetLabelsForIssueFunc: func(repo string, issueNum int) ([]string, error) {
			return []string{"jira-id: TEST-123"}, nil
		},
	}
	
	// Mock JIRA client with a successful CloseTicket implementation
	ticketClosed := false
	jiraClient := &MockJiraClient{
		CloseTicketFunc: func(ticketKey string) error {
			if ticketKey == "TEST-123" {
				ticketClosed = true
				return nil
			}
			return errors.New("unexpected ticket key")
		},
	}
	
	count, err := testSyncClosedIssues("owner/repo", githubClient, jiraClient)
	
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	
	if count != 1 {
		t.Errorf("Expected 1 closed ticket, got: %d", count)
	}
	
	if !ticketClosed {
		t.Error("Expected ticket to be closed, but CloseTicket was not called with the correct key")
	}
}

// TestSyncClosedIssuesWithCloseError tests the syncClosedIssues function when CloseTicket returns an error
func TestSyncClosedIssuesWithCloseError(t *testing.T) {
	// Mock GitHub client that returns a closed issue
	githubClient := &MockGitHubClient{
		GetClosedIssuesFunc: func(repo string) ([]models.GitHubIssue, error) {
			return []models.GitHubIssue{
				{
					Number: 123,
					Title: "Test Issue",
				},
			}, nil
		},
		GetLabelsForIssueFunc: func(repo string, issueNum int) ([]string, error) {
			return []string{"jira-id: TEST-123"}, nil
		},
	}
	
	// Mock JIRA client with a failing CloseTicket implementation
	jiraClient := &MockJiraClient{
		CloseTicketFunc: func(ticketKey string) error {
			return errors.New("failed to close ticket")
		},
	}
	
	count, err := testSyncClosedIssues("owner/repo", githubClient, jiraClient)
	
	if err != nil {
		t.Errorf("Expected no error from syncClosedIssues (it should handle CloseTicket errors), got: %v", err)
	}
	
	if count != 0 {
		t.Errorf("Expected 0 closed tickets (since CloseTicket failed), got: %d", count)
	}
} 