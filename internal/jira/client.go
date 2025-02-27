package jira

import (
	"fmt"
	"log"
	"os"
	"strings"

	jira "github.com/andygrunwald/go-jira"
	"github.com/dolaszy/glue/pkg/models"
)

// Client handles interactions with the JIRA API
type Client struct {
	client *jira.Client
}

// NewClient creates a new JIRA client
func NewClient() *Client {
	// Get JIRA URL, username, and API token from environment variables
	jiraURL := os.Getenv("JIRA_URL")
	username := os.Getenv("JIRA_USERNAME")
	token := os.Getenv("JIRA_TOKEN")

	if jiraURL == "" || username == "" || token == "" {
		log.Println("Warning: JIRA_URL, JIRA_USERNAME, and/or JIRA_TOKEN environment variables not set")
	}

	// Create JIRA authentication transport
	tp := jira.BasicAuthTransport{
		Username: username,
		Password: token,
	}

	// Create JIRA client
	client, err := jira.NewClient(tp.Client(), jiraURL)
	if err != nil {
		log.Printf("Error creating JIRA client: %v", err)
		return &Client{client: nil}
	}

	return &Client{
		client: client,
	}
}

// CreateTicket creates a JIRA ticket from a GitHub issue
func (c *Client) CreateTicket(boardName string, issue models.GitHubIssue) (string, error) {
	if c.client == nil {
		return "", fmt.Errorf("JIRA client not initialized")
	}

	// Get the project key from the board name
	// This assumes board names follow the format "PROJECT_KEY Board"
	// Adjust as needed for your JIRA setup
	projectKey := boardName
	if strings.Contains(boardName, " ") {
		projectKey = strings.Split(boardName, " ")[0]
	}

	// Determine issue type based on GitHub issue labels
	issueType := "Task" // Default issue type
	if issue.Type == "story" {
		issueType = "Story"
	} else if issue.Type == "feature" {
		issueType = "Feature"
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

// GetBoardStats returns statistics about a JIRA board
func (c *Client) GetBoardStats(boardName string) (int, int, error) {
	if c.client == nil {
		return 0, 0, fmt.Errorf("JIRA client not initialized")
	}

	// Get the project key from the board name
	projectKey := boardName
	if strings.Contains(boardName, " ") {
		projectKey = strings.Split(boardName, " ")[0]
	}

	// Search for all issues in the project
	jql := fmt.Sprintf("project = '%s'", projectKey)
	issues, resp, err := c.client.Issue.Search(jql, &jira.SearchOptions{MaxResults: 1000})
	if err != nil {
		return 0, 0, fmt.Errorf("failed to search JIRA issues: %v (status: %d)", err, resp.StatusCode)
	}

	// Count total and glued issues
	totalIssues := len(issues)
	gluedIssues := 0

	for _, issue := range issues {
		// Check if the issue was created by glue (look for the signature in description)
		if issue.Fields.Description != "" &&
			strings.Contains(issue.Fields.Description, "Created by glue from GitHub issue") {
			gluedIssues++
		}
	}

	return totalIssues, gluedIssues, nil
}
