package cmd

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/danielolaszy/glue/internal/github"
	"github.com/danielolaszy/glue/internal/jira"
	"github.com/danielolaszy/glue/internal/logging"
	"github.com/danielolaszy/glue/pkg/models"
	"github.com/spf13/cobra"
	"github.com/danielolaszy/glue/internal/config"

)

// jiraCmd represents the command to synchronize GitHub issues with JIRA.
// It creates JIRA tickets for GitHub issues labeled with the specified JIRA project(s).
var jiraCmd = &cobra.Command{
	Use:   "jira",
	Short: "Synchronize GitHub with JIRA",
	Long: `Synchronize GitHub issues with JIRA boards.

This command will create JIRA tickets for GitHub issues labeled with the specified JIRA project(s).
You can specify multiple boards using -b/--board flag multiple times.

Example:
  glue jira -r owner/repo -b PROJ1 -b PROJ2

Issues will be categorized based on their labels:
- GitHub issues with a 'feature' label will be created as 'Feature' type in JIRA
- GitHub issues with a 'story' label will be created as 'Story' type in JIRA
- Other GitHub issues will be created as 'Story' type in JIRA by default`,
	RunE: func(cmd *cobra.Command, args []string) error {
		repository, err := cmd.Flags().GetString("repository")
		if err != nil {
			return err
		}

		boards, err := cmd.Flags().GetStringArray("board")
		if err != nil {
			return err
		}

		if repository == "" {
			return fmt.Errorf("repository flag is required")
		}

		if len(boards) == 0 {
			return fmt.Errorf("at least one JIRA board must be specified using --board")
		}

		logging.Info("starting synchronization",
			"repository", repository,
			"boards", boards)

		// Initialize clients
		githubClient, err := github.NewClient()
		if err != nil {
			return fmt.Errorf("failed to initialize github client: %v", err)
		}

		jiraClient, err := jira.NewClient()
		if err != nil {
			return fmt.Errorf("failed to initialize jira client: %v", err)
		}

		// Get all issues for all boards in a single query
		issues, err := githubClient.GetIssuesWithLabels(repository, boards)
		if err != nil {
			return fmt.Errorf("failed to fetch github issues: %v", err)
		}

		// Also get closed issues for relationship mapping
		closedIssues, err := githubClient.GetClosedIssuesWithLabels(repository, boards)
		if err != nil {
			logging.Warn("failed to fetch closed github issues for relationships",
				"error", err)
		} else {
			// Combine open and closed issues for processing
			issues = append(issues, closedIssues...)
			logging.Debug("combined issues for processing",
				"open_count", len(issues)-len(closedIssues),
				"closed_count", len(closedIssues),
				"total_count", len(issues))
		}

		logging.Info("found github issues",
			"total_count", len(issues),
			"boards", boards)

		// Group issues by board
		issuesByBoard := make(map[string][]models.GitHubIssue)
		for _, issue := range issues {
			for _, board := range boards {
				if hasLabel(issue.Labels, board) {
					issuesByBoard[board] = append(issuesByBoard[board], issue)
					logging.Debug("assigned issue to board",
						"issue", issue.Number,
						"board", board,
						"title", issue.Title)
				}
			}
		}

		// Process each board with its pre-filtered issues
		totalSynced := 0
		for _, board := range boards {
			boardIssues := issuesByBoard[board]
			logging.Info("processing board",
				"board", board,
				"issue_count", len(boardIssues))

			if len(boardIssues) == 0 {
				logging.Warn("no issues found for board", "board", board)
				continue
			}

			syncCount, err := processBoard(repository, board, boardIssues, githubClient, jiraClient)
			if err != nil {
				logging.Error("error processing board",
					"board", board,
					"error", err)
				continue
			}

			totalSynced += syncCount
		}

		// After all boards are processed, check and update hierarchies
		logging.Info("checking issue hierarchies")
		for _, board := range boards {
			err := establishHierarchies(context.Background(), githubClient, jiraClient, repository, board, issuesByBoard[board])
			if err != nil {
				logging.Error("failed to establish hierarchies for board",
					"board", board,
					"error", err)
				continue
			}
		}

		// Process all closed issues once
		closeCount, err := syncClosedIssues(repository, githubClient, jiraClient)
		if err != nil {
			logging.Error("failed to sync closed issues",
				"error", err)
		} else if closeCount > 0 {
			logging.Info("closed jira tickets",
				"count", closeCount)
		}

		logging.Info("synchronization complete",
			"total_synchronized", totalSynced,
			"boards_processed", len(boards))

		return nil
	},
}

func init() {
	rootCmd.AddCommand(jiraCmd)
	jiraCmd.Flags().StringArrayP("board", "b", []string{}, "JIRA project board(s) to sync with (can be specified multiple times)")
}

// processBoard handles all operations for a single board
func processBoard(repository string, board string, issues []models.GitHubIssue, githubClient *github.Client, jiraClient *jira.Client) (int, error) {
	// Get issue type IDs once for this board
	featureTypeID, err := jiraClient.GetIssueTypeID(board, "feature")
	if err != nil {
		return 0, fmt.Errorf("failed to get 'feature' type ID: %v", err)
	}

	storyTypeID, err := jiraClient.GetIssueTypeID(board, "story")
	if err != nil {
		logging.Warn("failed to get 'story' type ID, using feature type",
			"board", board)
		storyTypeID = featureTypeID
	}

	// Group issues by type
	var features, stories, others []models.GitHubIssue
	for _, issue := range issues {
		if hasJiraIDPrefix(issue.Title) {
			continue // Skip already synced issues
		}

		if hasLabel(issue.Labels, "feature") {
			features = append(features, issue)
		} else if hasLabel(issue.Labels, "story") {
			stories = append(stories, issue)
		} else {
			others = append(others, issue)
		}
	}

	totalSyncCount := 0
	var allUpdatedIssues []models.GitHubIssue

	// Process features
	updatedFeatures, syncCount, err := processIssueGroup(features, featureTypeID, board, repository, githubClient, jiraClient)
	if err != nil {
		logging.Error("error processing features", "error", err)
	} else {
		totalSyncCount += syncCount
		allUpdatedIssues = append(allUpdatedIssues, updatedFeatures...)
	}

	// Process stories and others
	for _, group := range []struct {
		issues []models.GitHubIssue
		typeID string
	}{
		{stories, storyTypeID},
		{others, storyTypeID},
	} {
		updatedIssues, syncCount, err := processIssueGroup(group.issues, group.typeID, board, repository, githubClient, jiraClient)
		if err != nil {
			logging.Error("error processing issues", "error", err)
			continue
		}
		totalSyncCount += syncCount
		allUpdatedIssues = append(allUpdatedIssues, updatedIssues...)
	}

	// Process hierarchies
	if len(allUpdatedIssues) > 0 {
		if err := establishHierarchies(context.Background(), githubClient, jiraClient, repository, board, allUpdatedIssues); err != nil {
			logging.Error("error establishing hierarchies",
				"board", board,
				"error", err)
		}
	}

	return totalSyncCount, nil
}

// Helper functions
func hasJiraIDPrefix(title string) bool {
	return regexp.MustCompile(`^\[[A-Z]+-\d+\]`).MatchString(title)
}

func hasLabel(labels []string, targetLabel string) bool {
	for _, label := range labels {
		if strings.EqualFold(label, targetLabel) {
			return true
		}
	}
	return false
}

func parseJiraIDFromTitle(title string) string {
	re := regexp.MustCompile(`^\[([\w\-]+)\]`)
	matches := re.FindStringSubmatch(title)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func findIssuesSection(description string) string {
	parts := strings.Split(description, "## Issues")
	if len(parts) < 2 {
		return ""
	}

	nextSectionIdx := strings.Index(parts[1], "## ")
	if nextSectionIdx != -1 {
		return parts[1][:nextSectionIdx]
	}
	return parts[1]
}

func parseChildIssues(description string, gitHubDomain string) []int {
	var childNums []int
	issuesSection := findIssuesSection(description)
	if issuesSection == "" {
		return childNums
	}

	logging.Debug("found '## issues' section")

	escapedDomain := regexp.QuoteMeta(gitHubDomain)
	pattern := fmt.Sprintf(`https://%s/[^/]+/[^/]+/issues/(\d+)`, escapedDomain)
	re := regexp.MustCompile(pattern)
	matches := re.FindAllStringSubmatch(issuesSection, -1)

	for _, match := range matches {
		if len(match) > 1 {
			if num, err := strconv.Atoi(match[1]); err == nil {
				childNums = append(childNums, num)
			}
		}
	}

	logging.Debug("parsed child issues",
		"count", len(childNums),
		"issues", childNums)

	return childNums
}

// processIssueGroup handles creation of JIRA tickets for a group of issues
func processIssueGroup(issues []models.GitHubIssue, typeID string, board string, repository string, githubClient *github.Client, jiraClient *jira.Client) ([]models.GitHubIssue, int, error) {
	var updatedIssues []models.GitHubIssue
	syncCount := 0

	for _, issue := range issues {
		ticketID, err := jiraClient.CreateTicketWithTypeID(board, issue, typeID)
		if err != nil {
			logging.Error("failed to create ticket",
				"issue_number", issue.Number,
				"error", err)
			continue
		}

		newTitle := fmt.Sprintf("[%s] %s", ticketID, issue.Title)
		err = githubClient.UpdateIssueTitle(repository, issue.Number, newTitle)
		if err != nil {
			logging.Error("failed to update github issue title",
				"issue_number", issue.Number,
				"error", err)
			continue
		}

		updatedIssue, err := githubClient.GetIssue(repository, issue.Number)
		if err != nil {
			logging.Error("failed to fetch updated issue",
				"issue_number", issue.Number,
				"error", err)
			continue
		}

		updatedIssues = append(updatedIssues, updatedIssue)
		syncCount++
	}

	return updatedIssues, syncCount, nil
}

// buildGitHubToJiraMap creates a mapping of GitHub issue numbers to JIRA IDs
func buildGitHubToJiraMap(issues []models.GitHubIssue) map[int]string {
	githubToJira := make(map[int]string)
	for _, issue := range issues {
		if jiraID := parseJiraIDFromTitle(issue.Title); jiraID != "" {
			githubToJira[issue.Number] = jiraID
			logging.Debug("mapped github issue to jira",
				"github_number", issue.Number,
				"jira_id", jiraID)
		}
	}
	return githubToJira
}

// processFeatureLinks handles the creation and maintenance of parent-child relationships
func processFeatureLinks(feature models.GitHubIssue, githubToJira map[int]string, jiraClient *jira.Client, gitHubDomain string) (int, int, error) {
	linksCreated := 0
	linksRemoved := 0

	parentJiraID := parseJiraIDFromTitle(feature.Title)
	if parentJiraID == "" {
		return 0, 0, nil
	}

	childNums := parseChildIssues(feature.Description, gitHubDomain)
	if len(childNums) == 0 {
		return 0, 0, nil
	}

	logging.Debug("found child issues in feature description",
		"parent_jira", parentJiraID,
		"child_count", len(childNums),
		"github_domain", gitHubDomain)

	existingLinks, err := jiraClient.GetIssueLinks(parentJiraID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get existing links: %v", err)
	}

	validChildren := make(map[string]bool)
	for _, num := range childNums {
		childJiraID, exists := githubToJira[num]
		if !exists {
			logging.Debug("no JIRA ID found for GitHub issue",
				"github_number", num)
			continue
		}

		validChildren[childJiraID] = true

		if !existingLinks[childJiraID] {
			err := jiraClient.CreateParentChildLink(parentJiraID, childJiraID)
			if err != nil {
				logging.Error("failed to create parent-child link",
					"error", err,
					"parent", parentJiraID,
					"child", childJiraID)
			} else {
				linksCreated++
			}
		}
	}

	// Remove invalid links
	for childID := range existingLinks {
		if !validChildren[childID] {
			err := jiraClient.DeleteIssueLink(parentJiraID, childID)
			if err != nil {
				logging.Error("failed to remove parent-child link",
					"error", err,
					"parent", parentJiraID,
					"child", childID)
			} else {
				linksRemoved++
			}
		}
	}

	return linksCreated, linksRemoved, nil
}

// establishHierarchies manages the parent-child relationships between issues
func establishHierarchies(ctx context.Context, ghClient *github.Client, jiraClient *jira.Client, repository string, board string, issues []models.GitHubIssue) error {
	// Get config for GitHub domain
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}

	// Get all issues (open and closed) for mapping
	allIssues := make([]models.GitHubIssue, len(issues))
	copy(allIssues, issues)

	closedIssues, err := ghClient.GetClosedIssuesWithLabels(repository, []string{board})
	if err != nil {
		logging.Warn("failed to fetch closed issues for hierarchy mapping",
			"error", err,
			"board", board)
	} else {
		allIssues = append(allIssues, closedIssues...)
	}

	// Build GitHub to JIRA mapping
	githubToJira := buildGitHubToJiraMap(allIssues)

	totalLinksCreated := 0
	totalLinksRemoved := 0

	// Process each feature
	for _, issue := range issues {
		if !hasLabel(issue.Labels, "feature") {
			continue
		}

		created, removed, err := processFeatureLinks(issue, githubToJira, jiraClient, cfg.GitHub.Domain)
		if err != nil {
			logging.Error("error processing feature links",
				"error", err,
				"feature", issue.Number)
			continue
		}

		totalLinksCreated += created
		totalLinksRemoved += removed
	}

	logging.Info("parent-child relationship synchronization complete",
		"board", board,
		"relationships_created", totalLinksCreated,
		"relationships_removed", totalLinksRemoved)

	return nil
}

// syncClosedIssues handles synchronization of closed GitHub issues to JIRA
func syncClosedIssues(repository string, githubClient *github.Client, jiraClient *jira.Client) (int, error) {
	logging.Info("checking for closed github issues", "repository", repository)

	closedIssues, err := githubClient.GetClosedIssues(repository)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch closed GitHub issues: %v", err)
	}

	closeCount := 0
	for _, issue := range closedIssues {
		jiraID := parseJiraIDFromTitle(issue.Title)
		if jiraID == "" {
			continue
		}

		status, err := jiraClient.GetTicketStatus(jiraID)
		if err != nil {
			logging.Error("failed to get jira ticket status",
				"issue_number", issue.Number,
				"jira_ticket", jiraID,
				"error", err)
			continue
		}

		if status == "Done" {
			continue
		}

		err = jiraClient.CloseTicket(jiraID)
		if err != nil {
			logging.Error("failed to close jira ticket",
				"issue_number", issue.Number,
				"jira_ticket", jiraID,
				"error", err)
			continue
		}

		closeCount++
	}

	return closeCount, nil
}