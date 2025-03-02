// Package github provides functionality for interacting with the GitHub API.
package github

import (
	"context"

	"fmt"
	"regexp"
	"strings"
	"time"
	"net/url"

	"github.com/danielolaszy/glue/pkg/models"
	"github.com/google/go-github/v41/github"
	"golang.org/x/oauth2"
	"github.com/danielolaszy/glue/internal/config"
	"github.com/danielolaszy/glue/internal/logging"
)

// Client encapsulates the GitHub API client.
type Client struct {
	client *github.Client
}

// NewClient creates a new GitHub API client.
func NewClient() (*Client, error) {
	config, err := config.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	// Validate GitHub configuration
	token := config.GitHub.Token
	if token == "" {
		return nil, fmt.Errorf("github token not found in configuration")
	}

	baseURL := config.GitHub.BaseURL
	logging.Info("github configuration", 
		"base_url", baseURL, 
		"token_length", len(token))

	// Create the oauth2 client
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(context.Background(), ts)
	
	// Create GitHub client with custom base URL if provided
	var client *github.Client
	
	if baseURL == "https://api.github.com" {
		// Use default client for GitHub.com
		client = github.NewClient(tc)
	} else {
		// For custom GitHub instances (e.g., GitHub Enterprise)
		// Ensure baseURL ends with a slash
		if !strings.HasSuffix(baseURL, "/") {
			baseURL += "/"
		}
		
		// Parse the URL
		parsedURL, err := url.Parse(baseURL)
		if err != nil {
			return nil, fmt.Errorf("invalid github base url: %w", err)
		}
		
		// Create client with custom endpoint
		client = github.NewClient(tc)
		client.BaseURL = parsedURL
		
		// For GitHub Enterprise, we need to set the upload URL as well
		uploadURL, err := url.Parse(baseURL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse upload url: %w", err)
		}
		client.UploadURL = uploadURL
	}

	// Test the token
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	user, resp, err := client.Users.Get(ctx, "")
	if err != nil {
		logging.Error("failed to test github token", 
			"error", err, 
			"status_code", resp.StatusCode)
		return nil, fmt.Errorf("error testing github token: %w", err)
	}

	logging.Info("github authentication successful", 
		"username", *user.Login)

	return &Client{client: client}, nil
}

// GetAllIssues retrieves all open issues from a GitHub repository.
//
// Parameters:
//   - repository: The GitHub repository in the format "owner/repo"
//
// Returns:
//   - A slice of GitHubIssue objects representing the open issues
//   - An error if the issues couldn't be retrieved
func (c *Client) GetAllIssues(repository string) ([]models.GitHubIssue, error) {
	// Parse repository owner and name
	parts := strings.Split(repository, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repository format: %s, expected format: owner/repo", repository)
	}
	owner, repo := parts[0], parts[1]

	// Context for API requests
	ctx := context.Background()

	// Get all open issues
	opts := &github.IssueListByRepoOptions{
		State: "open",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	var allIssues []*github.Issue
	for {
		issues, resp, err := c.client.Issues.ListByRepo(ctx, owner, repo, opts)
		if err != nil {
			logging.Error("failed to fetch github issues", "error", err)
			return nil, fmt.Errorf("failed to fetch GitHub issues: %v", err)
		}

		allIssues = append(allIssues, issues...)

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	// Filter out pull requests and convert to our internal model
	var result []models.GitHubIssue
	for _, issue := range allIssues {
		// Skip pull requests (they're also returned by the Issues API)
		if issue.PullRequestLinks != nil {
			continue
		}

		// Convert to our internal model
		labelNames := make([]string, 0, len(issue.Labels))
		for _, label := range issue.Labels {
			labelNames = append(labelNames, *label.Name)
		}

		description := ""
		if issue.Body != nil {
			description = *issue.Body
		}

		result = append(result, models.GitHubIssue{
			Number:      *issue.Number,
			Title:       *issue.Title,
			Description: description,
			Labels:      labelNames,
		})
	}

	return result, nil
}

// AddLabels adds one or more labels to a GitHub issue. If the labels don't exist
// in the repository, they will be automatically created by GitHub.
//
// Parameters:
//   - repository: The GitHub repository in the format "owner/repo"
//   - issueNumber: The number of the issue to add labels to
//   - labels: One or more label strings to add to the issue
//
// Returns:
//   - An error if the labels couldn't be added
func (c *Client) AddLabels(repository string, issueNumber int, labels ...string) error {
	// Parse repository owner and name
	parts := strings.Split(repository, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid repository format: %s, expected format: owner/repo", repository)
	}
	owner, repo := parts[0], parts[1]

	// Context for API requests
	ctx := context.Background()

	// Log the operation
	logging.Info("adding labels", "labels", labels, "issue_number", issueNumber)

	// Add the labels to the issue
	// GitHub will automatically create labels that don't exist
	_, _, err := c.client.Issues.AddLabelsToIssue(ctx, owner, repo, issueNumber, labels)

	// Check for errors
	if err != nil {
		logging.Error("error adding labels to issue", "repository", repository, "issue_number", issueNumber, "error", err)
		return fmt.Errorf("failed to add labels to issue %s#%d: %v", repo, issueNumber, err)
	}

	logging.Info("successfully added labels", "labels", labels, "repository", repository, "issue_number", issueNumber)
	return nil
}

// GetLabelsForIssue retrieves all labels for a specific GitHub issue.
//
// Parameters:
//   - repository: The GitHub repository in the format "owner/repo"
//   - issueNumber: The number of the issue to get labels for
//
// Returns:
//   - A slice of strings representing the label names
//   - An error if the labels couldn't be retrieved
func (c *Client) GetLabelsForIssue(repository string, issueNumber int) ([]string, error) {
	// Parse repository owner and name
	parts := strings.Split(repository, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repository format: %s, expected format: owner/repo", repository)
	}
	owner, repo := parts[0], parts[1]

	// Context for API requests
	ctx := context.Background()

	// Log the operation
	logging.Info("retrieving labels", "repository", repository, "issue_number", issueNumber)

	// Get the labels for the issue
	// The GitHub API returns an array of label objects
	labels, _, err := c.client.Issues.ListLabelsByIssue(ctx, owner, repo, issueNumber, nil)

	// Check for errors
	if err != nil {
		logging.Error("error retrieving labels", "repository", repository, "issue_number", issueNumber, "error", err)
		return nil, fmt.Errorf("failed to retrieve labels for issue %s#%d: %v", repo, issueNumber, err)
	}

	// Convert the GitHub label objects to an array of strings
	// Each GitHub label object contains Name, Color, and Description fields
	labelNames := make([]string, len(labels))
	for i, label := range labels {
		labelNames[i] = label.GetName()
	}

	logging.Info("successfully retrieved labels", "repository", repository, "issue_number", issueNumber, "number_of_labels", len(labelNames))
	return labelNames, nil
}

// HasLabel checks if a GitHub issue has a specific label.
//
// Parameters:
//   - repository: The GitHub repository in the format "owner/repo"
//   - issueNumber: The number of the issue to check
//   - labelName: The exact name of the label to check for
//
// Returns:
//   - true if the issue has the label, false otherwise
//   - An error if there was a problem checking the labels
func (c *Client) HasLabel(repository string, issueNumber int, labelName string) (bool, error) {
	// Get all labels for the issue
	labels, err := c.GetLabelsForIssue(repository, issueNumber)
	if err != nil {
		return false, err
	}

	// Check if the specific label exists in the list
	for _, label := range labels {
		if label == labelName {
			return true, nil
		}
	}

	return false, nil
}

// HasLabelMatching checks if a GitHub issue has any label matching the given pattern.
//
// Parameters:
//   - repository: The GitHub repository in the format "owner/repo"
//   - issueNumber: The number of the issue to check
//   - pattern: A compiled regular expression pattern to match against label names
//
// Returns:
//   - true if any label matches the pattern, false otherwise
//   - An error if there was a problem checking the labels
func (c *Client) HasLabelMatching(repository string, issueNumber int, pattern *regexp.Regexp) (bool, error) {
	// Get all labels for the issue
	labels, err := c.GetLabelsForIssue(repository, issueNumber)
	if err != nil {
		return false, err
	}

	// Check if any label matches the pattern
	for _, label := range labels {
		if pattern.MatchString(label) {
			return true, nil
		}
	}

	return false, nil
}
