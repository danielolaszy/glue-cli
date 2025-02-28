// Package jira provides functionality for interacting with the JIRA API.
package jira

import (
	"fmt"
	"io"
	"os"

	jira "github.com/andygrunwald/go-jira"
	"github.com/danielolaszy/glue/pkg/models"
	"github.com/danielolaszy/glue/internal/logging"

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
	
	logging.Info("jira configuration", 
		"base_url", jiraURL, 
		"username", jiraUsername, 
		"token_length", len(jiraToken))
	
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
		logging.Error("failed to create jira client", "error", err)
		return nil, fmt.Errorf("error creating jira client: %v", err)
	}
	
	// Verify client is properly initialized
	if client == nil {
		return nil, fmt.Errorf("jira client is nil after initialization")
	}
	
	if client.Issue == nil {
		return nil, fmt.Errorf("jira client Issue service is nil")
	}
	
	// Test the connection by getting the current user
	myself, resp, err := client.User.GetSelf()
	if err != nil {
		logging.Error("failed to test jira connection", "error", err, "status_code", resp.StatusCode)
		return nil, fmt.Errorf("error testing jira connection: %v", err)
	}
	logging.Info("jira authentication successful", "username", myself.EmailAddress)
	
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
		return "", fmt.Errorf("jira client not initialized")
	}

	// First, let's verify the project exists and get available issue types
	project, resp, err := c.client.Project.Get(projectKey)
	if err != nil {
		logging.Error("failed to get jira project", 
			"project_key", projectKey, 
			"error", err, 
			"status_code", resp.StatusCode)
		return "", fmt.Errorf("failed to get jira project '%s': %v (status: %d)", 
			projectKey, err, resp.StatusCode)
	}
	
	// Debug: Print available issue types
	logging.Debug("available issue types", "project", projectKey)
	validIssueType := false
	var availableTypes []string
	
	for _, issueTypeObj := range project.IssueTypes {
		availableTypes = append(availableTypes, issueTypeObj.Name)
		logging.Debug("issue type", "name", issueTypeObj.Name, "id", issueTypeObj.ID)
		if issueTypeObj.Name == issueType {
			validIssueType = true
		}
	}
	
	// If the requested issue type doesn't exist, use the first available type
	if !validIssueType {
		if len(availableTypes) > 0 {
			logging.Warn("issue type not found, using alternative", 
				"requested_type", issueType, 
				"using_type", availableTypes[0])
			issueType = availableTypes[0]
		} else {
			logging.Error("no issue types available in project", "project_key", projectKey)
			return "", fmt.Errorf("no issue types available in project '%s'", projectKey)
		}
	}

	// Prepare issue fields
	description := fmt.Sprintf("%s\n\n----\nCreated by glue from GitHub issue #%d",
		issue.Description, issue.Number)

	logging.Info("creating jira ticket", 
		"project", projectKey, 
		"type", issueType, 
		"title", issue.Title)
	
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

	logging.Debug("sending request to jira api")
	
	newIssue, resp, err := c.client.Issue.Create(jiraIssue)
	if err != nil {
		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
			
			// Try to get more details about the error
			body, readErr := io.ReadAll(resp.Body)
			if readErr == nil {
				logging.Error("failed to create jira ticket", 
					"error", err, 
					"status_code", statusCode, 
					"response", string(body))
				return "", fmt.Errorf("failed to create jira ticket: %v (status: %d, response: %s)", 
					err, statusCode, string(body))
			}
		}
		logging.Error("failed to create jira ticket", "error", err, "status_code", statusCode)
		return "", fmt.Errorf("failed to create jira ticket: %v (status: %d)", err, statusCode)
	}

	if newIssue == nil {
		logging.Error("jira api returned nil issue")
		return "", fmt.Errorf("jira api returned nil issue")
	}

	logging.Info("created jira ticket", "key", newIssue.Key)
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
		return 0, fmt.Errorf("jira client not initialized")
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
		return 0, fmt.Errorf("failed to search jira issues: %v (status: %d)", err, resp.StatusCode)
	}

	return len(result), nil
}
