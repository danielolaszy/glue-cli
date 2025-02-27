package github

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/dolaszy/glue/pkg/models"
	"github.com/google/go-github/v41/github"
	"golang.org/x/oauth2"
)

// Client handles interactions with the GitHub API
type Client struct {
	client *github.Client
}

// NewClient creates a new GitHub client
func NewClient() *Client {
	// Get GitHub token from environment variable
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Println("Warning: GITHUB_TOKEN environment variable not set")
	}

	// Create OAuth2 client
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)

	// Create GitHub client
	client := github.NewClient(tc)

	return &Client{
		client: client,
	}
}

// InitializeLabels creates the required labels in the repository if they don't exist
func (c *Client) InitializeLabels(repository string) error {
	// Parse repository owner and name
	parts := strings.Split(repository, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid repository format: %s, expected format: owner/repo", repository)
	}
	owner, repo := parts[0], parts[1]

	// Required labels
	requiredLabels := []string{"story", "feature", "glued"}

	// Context for API requests
	ctx := context.Background()

	// Check and create each label
	for _, label := range requiredLabels {
		// Try to get the label
		_, resp, err := c.client.Issues.GetLabel(ctx, owner, repo, label)

		if err != nil && resp.StatusCode == 404 {
			// Label doesn't exist, create it
			color := getLabelColor(label)
			description := getLabelDescription(label)

			newLabel := &github.Label{
				Name:        github.String(label),
				Color:       github.String(color),
				Description: github.String(description),
			}

			_, _, err := c.client.Issues.CreateLabel(ctx, owner, repo, newLabel)
			if err != nil {
				return fmt.Errorf("failed to create label '%s': %v", label, err)
			}

			fmt.Printf("Created label '%s'\n", label)
		} else if err != nil {
			return fmt.Errorf("error checking label '%s': %v", label, err)
		} else {
			fmt.Printf("Label '%s' already exists\n", label)
		}
	}

	return nil
}

// GetUnglued returns GitHub issues without the 'glued' label
func (c *Client) GetUnglued(repository string) ([]models.GitHubIssue, error) {
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
			return nil, fmt.Errorf("failed to fetch GitHub issues: %v", err)
		}

		allIssues = append(allIssues, issues...)

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	// Filter issues without the 'glued' label
	var result []models.GitHubIssue
	for _, issue := range allIssues {
		// Skip pull requests (they're also returned by the Issues API)
		if issue.PullRequestLinks != nil {
			continue
		}

		// Check if the issue has the 'glued' label
		hasGluedLabel := false
		issueType := ""

		for _, label := range issue.Labels {
			if *label.Name == "glued" {
				hasGluedLabel = true
			}
			if *label.Name == "story" {
				issueType = "story"
			}
			if *label.Name == "feature" {
				issueType = "feature"
			}
		}

		if !hasGluedLabel {
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
				Type:        issueType,
			})
		}
	}

	return result, nil
}

// AddGluedLabel adds the 'glued' label to a GitHub issue
func (c *Client) AddGluedLabel(repository string, issueNumber int) error {
	// Parse repository owner and name
	parts := strings.Split(repository, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid repository format: %s, expected format: owner/repo", repository)
	}
	owner, repo := parts[0], parts[1]

	// Context for API requests
	ctx := context.Background()

	// Add the 'glued' label
	_, _, err := c.client.Issues.AddLabelsToIssue(ctx, owner, repo, issueNumber, []string{"glued"})
	return err
}

// GetSyncStats returns statistics about synchronized and non-synchronized GitHub issues
func (c *Client) GetSyncStats(repository string) (int, int, error) {
	// Parse repository owner and name
	parts := strings.Split(repository, "/")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid repository format: %s, expected format: owner/repo", repository)
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
			return 0, 0, fmt.Errorf("failed to fetch GitHub issues: %v", err)
		}

		allIssues = append(allIssues, issues...)

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	// Count issues with and without the 'glued' label
	glued := 0
	unglued := 0

	for _, issue := range allIssues {
		// Skip pull requests
		if issue.PullRequestLinks != nil {
			continue
		}

		// Check if the issue has the 'glued' label
		hasGluedLabel := false
		for _, label := range issue.Labels {
			if *label.Name == "glued" {
				hasGluedLabel = true
				break
			}
		}

		if hasGluedLabel {
			glued++
		} else {
			unglued++
		}
	}

	return glued, unglued, nil
}

// Helper functions for label creation
func getLabelColor(label string) string {
	switch label {
	case "story":
		return "0075ca" // Blue
	case "feature":
		return "7057ff" // Purple
	case "glued":
		return "d4c5f9" // Light purple
	default:
		return "cccccc" // Light gray
	}
}

func getLabelDescription(label string) string {
	switch label {
	case "story":
		return "User story"
	case "feature":
		return "Feature request"
	case "glued":
		return "Synchronized with project management tool"
	default:
		return ""
	}
}
