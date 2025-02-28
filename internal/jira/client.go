// Package jira provides functionality for interacting with the JIRA API.
package jira

import (
	"fmt"
	"os"
	"strings"

	jira "github.com/andygrunwald/go-jira"
	"github.com/danielolaszy/glue/pkg/models"

)

// Client handles interactions with the JIRA API.
type Client struct {
	client *jira.Client
	BaseURL  string
	Username string
	Token    string
}

// NewClient creates a new JIRA API client instance.
func NewClient() (*Client, error) {
	// Get JIRA credentials from environment variables
	jiraURL := os.Getenv("JIRA_URL")
	jiraUsername := os.Getenv("JIRA_USERNAME")
	jiraToken := os.Getenv("JIRA_TOKEN")
	
	fmt.Printf("JIRA Configuration - URL: %s, Username: %s, Token: %s\n", 
		jiraURL, 
		jiraUsername, 
		strings.Repeat("*", len(jiraToken)))
	
	// Validate credentials
	var missingVars []string
	if jiraURL == "" {
		missingVars = append(missingVars, "JIRA_URL")
	}
	if jiraUsername == "" {
		missingVars = append(missingVars, "JIRA_USERNAME")
	}
	if jiraToken == "" {
		missingVars = append(missingVars, "JIRA_TOKEN")
	}
	
	if len(missingVars) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %v", missingVars)
	}
	
	// Create transport for authentication
	tp := jira.BasicAuthTransport{
		Username: jiraUsername,
		Password: jiraToken,
	}
	
	// Create JIRA client
	client, err := jira.NewClient(tp.Client(), jiraURL)
	if err != nil {
		return nil, fmt.Errorf("error creating JIRA client: %v", err)
	}
	
	// Verify client is properly initialized
	if client == nil {
		return nil, fmt.Errorf("JIRA client is nil after initialization")
	}
	
	if client.Issue == nil {
		return nil, fmt.Errorf("JIRA client Issue service is nil")
	}
	
	// Test the connection by getting the current user
	myself, _, err := client.User.GetSelf()
	if err != nil {
		return nil, fmt.Errorf("error testing JIRA connection: %v", err)
	}
	
	fmt.Printf("JIRA connection successful - Authenticated as: %s\n", myself.DisplayName)
	
	return &Client{
		client: client,
	}, nil
}

// CreateTicket creates a new JIRA ticket from a GitHub issue.
//
// Parameters:
//   - projectKey: The JIRA project key (e.g., "ABC" for tickets like "ABC-123")
//   - issue: The GitHub issue to create a ticket for
//   - issueType: The type of JIRA issue to create (e.g., "Story", "Feature", "Task")
//
// Returns:
//   - The ID of the created JIRA ticket (e.g., "ABC-123")
//   - An error if the ticket couldn't be created
func (c *Client) CreateTicket(projectKey string, issue models.GitHubIssue, issueType string) (string, error) {
	if c.client == nil {
		return "", fmt.Errorf("JIRA client not initialized")
	}

	// Prepare issue fields
	description := fmt.Sprintf("%s\n\n----\nCreated by glue from GitHub issue #%d",
		issue.Description, issue.Number)

	issueFields := &jira.IssueFields{
		Project: jira.Project{
			Key: projectKey,
		},
		Summary:     issue.Title,
		Description: description,
		Type: jira.IssueType{
			Name: issueType,
		},
	}

	// Create the issue
	jiraIssue := &jira.Issue{
		Fields: issueFields,
	}

	newIssue, resp, err := c.client.Issue.Create(jiraIssue)
	if err != nil {
		return "", fmt.Errorf("failed to create JIRA ticket: %v (status: %d)", err, resp.StatusCode)
	}

	return newIssue.Key, nil
}

// GetTotalTickets returns the total number of tickets in a JIRA project.
//
// Parameters:
//   - projectKey: The JIRA project key (e.g., "ABC" for tickets like "ABC-123")
//
// Returns:
//   - The number of tickets in the project
//   - An error if the count couldn't be retrieved
func (c *Client) GetTotalTickets(projectKey string) (int, error) {
	if c.client == nil {
		return 0, fmt.Errorf("JIRA client not initialized")
	}

	// Search for all issues in the project
	jql := fmt.Sprintf("project = '%s'", projectKey)

	// We're only interested in the count, not the actual issues
	options := &jira.SearchOptions{
		MaxResults: 0,              // We don't need actual results, just the count
		Fields:     []string{"id"}, // Minimize data returned
	}

	result, resp, err := c.client.Issue.Search(jql, options)
	if err != nil {
		return 0, fmt.Errorf("failed to search JIRA issues: %v (status: %d)", err, resp.StatusCode)
	}

	return len(result), nil
}
