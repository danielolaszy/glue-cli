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

// NewClient creates a new GitHub API client using configuration from environment variables.
// It initializes the client with the appropriate base URL, authenticates with the GitHub API,
// and tests the connection. It returns the configured client or an error if initialization fails.
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

	// Get domain from config, default to github.com
	domain := config.GitHub.Domain
	if domain == "" {
		domain = "github.com"
	}

	// Construct API URL based on domain
	var apiURL string
	if domain == "github.com" {
		apiURL = "https://api.github.com/"
	} else {
		apiURL = fmt.Sprintf("https://%s/api/v3/", domain)
	}

	logging.Info("github configuration", 
		"domain", domain, 
		"api_url", apiURL,
		"token_length", len(token))

	// Create the oauth2 client
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(context.Background(), ts)
	
	// Create GitHub client with custom base URL
	client := github.NewClient(tc)
	
	// If not using default GitHub.com, set custom API endpoint
	if domain != "github.com" {
		parsedURL, err := url.Parse(apiURL)
		if err != nil {
			return nil, fmt.Errorf("invalid github api url: %w", err)
		}
		
		client.BaseURL = parsedURL
		
		// For GitHub Enterprise, set the upload URL to the same endpoint
		client.UploadURL = parsedURL
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
// It filters out pull requests and converts the GitHub API objects to our internal model.
// The repository should be in the format "owner/repo". It returns a slice of issues
// or an error if the retrieval fails.
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
// in the repository, GitHub will automatically create them. The repository should be
// in the format "owner/repo". It returns an error if the operation fails.
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
	logging.Debug("adding labels", "labels", labels, "issue_number", issueNumber)

	// Add the labels to the issue
	// GitHub will automatically create labels that don't exist
	_, _, err := c.client.Issues.AddLabelsToIssue(ctx, owner, repo, issueNumber, labels)

	// Check for errors
	if err != nil {
		logging.Error("error adding labels to issue", "repository", repository, "issue_number", issueNumber, "error", err)
		return fmt.Errorf("failed to add labels to issue %s#%d: %v", repo, issueNumber, err)
	}

	logging.Debug("successfully added labels", "labels", labels, "repository", repository, "issue_number", issueNumber)
	return nil
}

// GetLabelsForIssue retrieves all labels for a specific GitHub issue and returns
// them as string names. The repository should be in the format "owner/repo".
// It returns a slice of label names or an error if the retrieval fails.
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
	logging.Debug("retrieving labels", "repository", repository, "issue_number", issueNumber)

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

	logging.Debug("successfully retrieved labels", "repository", repository, "issue_number", issueNumber, "number_of_labels", len(labelNames))
	return labelNames, nil
}

// HasLabel checks if a GitHub issue has a specific label using exact matching.
// The repository should be in the format "owner/repo". It returns true if the
// label is found, false otherwise, and any error encountered during checking.
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

// HasLabelMatching checks if a GitHub issue has any label matching a regular expression pattern.
// The repository should be in the format "owner/repo". It returns true if any label
// matches the pattern, false otherwise, and any error encountered during checking.
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

// IsIssueClosed checks if a GitHub issue is closed.
// The repository should be in the format "owner/repo". It returns true if the issue
// is closed, false if it's open, and any error encountered during checking.
func (c *Client) IsIssueClosed(repository string, issueNumber int) (bool, error) {
	// Parse repository owner and name
	parts := strings.Split(repository, "/")
	if len(parts) != 2 {
		return false, fmt.Errorf("invalid repository format: %s, expected format: owner/repo", repository)
	}
	owner, repo := parts[0], parts[1]

	// Context for API requests
	ctx := context.Background()

	// Get the issue
	issue, resp, err := c.client.Issues.Get(ctx, owner, repo, issueNumber)
	if err != nil {
		logging.Error("failed to get github issue", 
			"repository", repository, 
			"issue_number", issueNumber, 
			"error", err, 
			"status_code", resp.StatusCode)
		return false, fmt.Errorf("failed to get GitHub issue: %v", err)
	}

	// Check the state of the issue
	return *issue.State == "closed", nil
}

// GetClosedIssues retrieves all closed issues from a GitHub repository.
// It filters out pull requests and converts the GitHub API objects to our internal model.
// The repository should be in the format "owner/repo". It returns a slice of issues
// or an error if the retrieval fails.
func (c *Client) GetClosedIssues(repository string) ([]models.GitHubIssue, error) {
	// Parse repository owner and name
	parts := strings.Split(repository, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repository format: %s, expected format: owner/repo", repository)
	}
	owner, repo := parts[0], parts[1]

	// Context for API requests
	ctx := context.Background()

	// Get all closed issues
	opts := &github.IssueListByRepoOptions{
		State: "closed",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	var allIssues []*github.Issue
	for {
		issues, resp, err := c.client.Issues.ListByRepo(ctx, owner, repo, opts)
		if err != nil {
			logging.Error("failed to fetch closed github issues", "error", err)
			return nil, fmt.Errorf("failed to fetch GitHub closed issues: %v", err)
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
