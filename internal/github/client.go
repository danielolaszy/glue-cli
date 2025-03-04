// Package github provides functionality for interacting with the GitHub API.
package github

import (
	"context"

	"fmt"
	"regexp"
	"strings"
	"time"
	"os"
	"net/http"

	"github.com/danielolaszy/glue/pkg/models"
	"github.com/google/go-github/v41/github"
	"golang.org/x/oauth2"
	"github.com/danielolaszy/glue/internal/logging"
)

// Client encapsulates the GitHub API client.
type Client struct {
	client *github.Client
	ctx    context.Context
	cancel context.CancelFunc
}

// NewClient creates a new GitHub client with retries and longer timeout
func NewClient() (*Client, error) {
	// Get GitHub configuration
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN environment variable is required")
	}

	// Increase timeout to 30 seconds
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	
	// Create an HTTP client with longer timeouts
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	logging.Debug("initializing github client", 
		"token_length", len(token),
		"token_prefix", token[:5]+"...") // Only log first 5 chars for security

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	// Use our custom httpClient as the base client
	tc := oauth2.NewClient(ctx, ts)
	tc.Timeout = httpClient.Timeout

	client := github.NewClient(tc)

	// Test authentication
	maxRetries := 3
	var user *github.User
	var err error
	
	for attempt := 1; attempt <= maxRetries; attempt++ {
		logging.Debug("testing github authentication",
			"attempt", attempt)
		
		user, _, err = client.Users.Get(ctx, "")
		if err == nil {
			break
		}
		
		if attempt < maxRetries {
			logging.Warn("github authentication attempt failed, retrying...",
				"attempt", attempt,
				"error", err)
			time.Sleep(time.Second * time.Duration(attempt))
		}
	}

	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to authenticate with github: %v", err)
	}

	logging.Info("github authentication successful",
		"username", user.GetLogin())

	return &Client{
		client:  client,
		ctx:     ctx,
		cancel:  cancel,
	}, nil
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

// GetIssuesWithLabel retrieves all open issues that have a specific label
func (c *Client) GetIssuesWithLabel(repository, label string) ([]models.GitHubIssue, error) {
	logging.Debug("fetching github issues with label",
		"repository", repository,
		"label", label)

	query := fmt.Sprintf("repo:%s is:issue is:open label:%s", repository, label)
	opts := &github.SearchOptions{
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	var allIssues []models.GitHubIssue
	for {
		result, resp, err := c.client.Search.Issues(context.Background(), query, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to search issues: %v", err)
		}

		for _, issue := range result.Issues {
			labels := make([]string, 0, len(issue.Labels))
			for _, label := range issue.Labels {
				if label.Name != nil {
					labels = append(labels, *label.Name)
				}
			}

			allIssues = append(allIssues, models.GitHubIssue{
				Number:      *issue.Number,
				Title:       *issue.Title,
				Description: *issue.Body,
				State:       *issue.State,
				CreatedAt:   *issue.CreatedAt,
				UpdatedAt:   *issue.UpdatedAt,
				ClosedAt:    issue.ClosedAt,
				Labels:      labels,
			})
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return allIssues, nil
}

// UpdateIssueTitle updates the title of a GitHub issue
func (c *Client) UpdateIssueTitle(repository string, issueNumber int, newTitle string) error {
	parts := strings.Split(repository, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid repository format: %s", repository)
	}

	issue := &github.IssueRequest{
		Title: &newTitle,
	}

	_, _, err := c.client.Issues.Edit(context.Background(), parts[0], parts[1], issueNumber, issue)
	if err != nil {
		return fmt.Errorf("failed to update issue title: %v", err)
	}

	return nil
}

// GetIssue retrieves a specific GitHub issue by number
func (c *Client) GetIssue(repository string, issueNumber int) (models.GitHubIssue, error) {
	parts := strings.Split(repository, "/")
	if len(parts) != 2 {
		return models.GitHubIssue{}, fmt.Errorf("invalid repository format: %s", repository)
	}

	issue, _, err := c.client.Issues.Get(context.Background(), parts[0], parts[1], issueNumber)
	if err != nil {
		return models.GitHubIssue{}, fmt.Errorf("failed to get issue: %v", err)
	}

	labels := make([]string, 0, len(issue.Labels))
	for _, label := range issue.Labels {
		if label.Name != nil {
			labels = append(labels, *label.Name)
		}
	}

	return models.GitHubIssue{
		Number:      *issue.Number,
		Title:       *issue.Title,
		Description: *issue.Body,
		Labels:      labels,
	}, nil
}

// GetIssuesWithLabels retrieves all open issues with any of the specified labels
func (c *Client) GetIssuesWithLabels(repository string, labels []string) ([]models.GitHubIssue, error) {
	var allIssues []models.GitHubIssue
	
	// Start with just getting all open issues
	query := fmt.Sprintf("repo:%s is:issue is:open", repository)
	
	logging.Debug("searching for github issues", 
		"query", query)

	opts := &github.SearchOptions{
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	result, _, err := c.client.Search.Issues(c.ctx, query, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to search issues: %v", err)
	}

	logging.Debug("found issues without label filter",
		"total_count", result.GetTotal())

	// Now filter by labels in memory
	for _, issue := range result.Issues {
		issueLabels := extractLabelsFromIssue(issue)
		for _, targetLabel := range labels {
			if hasLabel(issueLabels, targetLabel) {
				ghIssue := models.GitHubIssue{
					Number:      issue.GetNumber(),
					Title:      issue.GetTitle(),
					Description: issue.GetBody(),
					Labels:     issueLabels,
					State:      issue.GetState(),
					CreatedAt:  issue.GetCreatedAt(),
					UpdatedAt:  issue.GetUpdatedAt(),
				}
				allIssues = append(allIssues, ghIssue)
				break // Found one matching label, no need to check others
			}
		}
	}

	logging.Debug("filtered issues by labels",
		"total_matching", len(allIssues),
		"labels", labels)

	return allIssues, nil
}

// Helper function to extract labels
func extractLabelsFromIssue(issue *github.Issue) []string {
	labels := make([]string, len(issue.Labels))
	for i, label := range issue.Labels {
		labels[i] = label.GetName()
	}
	return labels
}

// Add this helper function to the client package
func hasLabel(labels []string, targetLabel string) bool {
	for _, label := range labels {
		if strings.EqualFold(label, targetLabel) {
			return true
		}
	}
	return false
}
