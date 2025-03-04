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

	// Create tickets in batches
	syncCount := 0
	var updatedFeatures []models.GitHubIssue // Keep track of features with their updated titles
	
	// Process features first (they might be parents)
	for _, issue := range features {
		ticketID, err := jiraClient.CreateTicketWithTypeID(board, issue, featureTypeID)
		if err != nil {
			logging.Error("failed to create feature ticket",
				"issue_number", issue.Number,
				"error", err)
			continue
		}

		// Update GitHub issue title with JIRA ID
		newTitle := fmt.Sprintf("[%s] %s", ticketID, issue.Title)
		err = githubClient.UpdateIssueTitle(repository, issue.Number, newTitle)
		if err != nil {
			logging.Error("failed to update github issue title",
				"issue_number", issue.Number,
				"error", err)
			continue
		}

		// Get the updated issue for hierarchy processing
		updatedIssue, err := githubClient.GetIssue(repository, issue.Number)
		if err != nil {
			logging.Error("failed to fetch updated issue",
				"issue_number", issue.Number,
				"error", err)
			continue
		}

		updatedFeatures = append(updatedFeatures, updatedIssue)
		syncCount++
	}

	// Process stories and others with the same pattern
	var updatedStories []models.GitHubIssue
	for _, issueGroup := range []struct {
		issues []models.GitHubIssue
		typeID string
	}{
		{stories, storyTypeID},
		{others, storyTypeID},
	} {
		for _, issue := range issueGroup.issues {
			ticketID, err := jiraClient.CreateTicketWithTypeID(board, issue, issueGroup.typeID)
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

			// Get the updated issue
			updatedIssue, err := githubClient.GetIssue(repository, issue.Number)
			if err != nil {
				logging.Error("failed to fetch updated issue",
					"issue_number", issue.Number,
					"error", err)
				continue
			}

			updatedStories = append(updatedStories, updatedIssue)
			syncCount++
		}
	}

	// Process hierarchies after all tickets are created and updated
	allUpdatedIssues := append(updatedFeatures, updatedStories...)
	if len(allUpdatedIssues) > 0 {
		if err := establishHierarchies(context.Background(), githubClient, jiraClient, repository, board, allUpdatedIssues); err != nil {
			logging.Error("error establishing hierarchies",
				"board", board,
				"error", err)
		}
	}

	return syncCount, nil
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

// extractJiraProject searches for a label beginning with "jira-project:" and
// extracts the JIRA project name from it. It returns the project name and a
// boolean indicating whether the label was found.
func extractJiraProject(labels []string) (string, bool) {
	const prefix = "jira-project:"

	for _, label := range labels {
		if strings.HasPrefix(label, prefix) {
			// Extract the project name, which is after the prefix and a space
			parts := strings.SplitN(label, ":", 2)
			if len(parts) == 2 {
				boardName := strings.TrimSpace(parts[1])
				if boardName != "" {
					return boardName, true
				}
			}
		}
	}

	return "", false
}

// getJiraIDFromLabels searches for a label beginning with "jira-id:" and
// extracts the JIRA ticket ID from it. It returns the ticket ID if found
// or an empty string if no matching label exists.
func getJiraIDFromLabels(labels []string) string {
	const prefix = "jira-id:"

	for _, label := range labels {
		if strings.HasPrefix(label, prefix) {
			// Extract the JIRA ID, which is after the prefix and a space
			parts := strings.SplitN(label, ":", 2)
			if len(parts) == 2 {
				jiraID := strings.TrimSpace(parts[1])
				if jiraID != "" {
					return jiraID
				}
			}
		}
	}

	return ""
}

// syncGitHubToJira synchronizes GitHub issues with JIRA tickets for a specific board
func syncGitHubToJira(repository string, board string, githubClient *github.Client, jiraClient *jira.Client) (int, error) {
	logging.Info("syncing github to jira",
		"repository", repository,
		"board", board)

	// Get GitHub issues filtered by board label
	issues, err := githubClient.GetIssuesWithLabels(repository, []string{board})
	if err != nil {
		return 0, fmt.Errorf("failed to fetch github issues: %v", err)
	}

	// Process the board
	syncCount, err := processBoard(repository, board, issues, githubClient, jiraClient)
	if err != nil {
		return 0, fmt.Errorf("error processing board: %v", err)
	}

	logging.Info("github to jira sync completed",
		"repository", repository,
		"board", board,
		"tickets_created", syncCount)

	return syncCount, nil
}

// extractJiraIDFromTitle extracts the JIRA ID from a title if present
func extractJiraIDFromTitle(title string) (string, bool) {
	matches := regexp.MustCompile(`^\[([A-Z]+-\d+)\]`).FindStringSubmatch(title)
	if len(matches) > 1 {
		return matches[1], true
	}
	return "", false
}

// syncClosedIssues checks all GitHub issues that are closed and have JIRA IDs
// in their titles, then closes their corresponding JIRA tickets if they're not already closed.
func syncClosedIssues(repository string, githubClient *github.Client, jiraClient *jira.Client) (int, error) {
	logging.Info("checking for closed github issues", "repository", repository)

	// Get all closed issues from GitHub
	closedIssues, err := githubClient.GetClosedIssues(repository)
	if err != nil {
		logging.Error("failed to fetch closed github issues", "error", err)
		return 0, fmt.Errorf("failed to fetch closed GitHub issues: %v", err)
	}

	logging.Info("found closed github issues", 
		"count", len(closedIssues), 
		"repository", repository)
	
	closeCount := 0

	// Process each closed issue
	for _, issue := range closedIssues {
		// Extract JIRA ID from title
		jiraID := parseJiraIDFromTitle(issue.Title)
		if jiraID == "" {
			logging.Debug("closed issue has no JIRA ID in title, skipping",
				"repository", repository,
				"issue_number", issue.Number,
				"title", issue.Title)
			continue
		}

		// Check if the JIRA ticket is already closed
		status, err := jiraClient.GetTicketStatus(jiraID)
		if err != nil {
			logging.Error("failed to get jira ticket status",
				"repository", repository,
				"issue_number", issue.Number,
				"jira_ticket", jiraID,
				"error", err)
			continue
		}

		if status == "Done" {
			logging.Debug("jira ticket already closed, skipping",
				"repository", repository,
				"issue_number", issue.Number,
				"jira_ticket", jiraID)
			continue
		}

		// Close the corresponding JIRA ticket
		logging.Info("closing JIRA ticket for closed GitHub issue",
			"repository", repository,
			"issue_number", issue.Number,
			"jira_ticket", jiraID,
			"current_status", status)

		err = jiraClient.CloseTicket(jiraID)
		if err != nil {
			logging.Error("failed to close jira ticket",
				"repository", repository,
				"issue_number", issue.Number,
				"jira_ticket", jiraID,
				"error", err)
			continue
		}

		logging.Info("successfully closed JIRA ticket",
			"repository", repository,
			"issue_number", issue.Number,
			"jira_ticket", jiraID)

		closeCount++
	}

	if closeCount > 0 {
		logging.Info("closed jira tickets", "count", closeCount)
	} else {
		logging.Debug("no jira tickets needed closing")
	}

	return closeCount, nil
}

// parseChildIssues extracts GitHub issue numbers from the "## Issues" section
func parseChildIssues(description string, gitHubDomain string) []string {
	var childLinks []string

	// Find the "## Issues" section
	issuesSection := findIssuesSection(description)
	if issuesSection == "" {
		return childLinks
	}

	logging.Debug("found '## issues' section")

	// Extract GitHub issue numbers using regex
	escapedDomain := regexp.QuoteMeta(gitHubDomain)
	pattern := fmt.Sprintf(`https://%s/[^/]+/[^/]+/issues/(\d+)`, escapedDomain)
	re := regexp.MustCompile(pattern)
	matches := re.FindAllStringSubmatch(issuesSection, -1)

	// Extract just the issue numbers
	for _, match := range matches {
		if len(match) > 1 {
			childLinks = append(childLinks, match[1])
		}
	}

	logging.Debug("parsed child issues",
		"count", len(childLinks),
		"issues", childLinks)

	return childLinks
}

func establishHierarchies(ctx context.Context, ghClient *github.Client, jiraClient *jira.Client, repository string, board string, issues []models.GitHubIssue) error {
	log := logging.GetLogger()

	// Add counters for created and removed links
	linksCreated := 0
	linksRemoved := 0

	// Get config for GitHub domain
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}

	// Get all issues (open and closed) for this board to build complete mapping
	allIssues := make([]models.GitHubIssue, len(issues))
	copy(allIssues, issues)

	// Get closed issues for this board
	closedIssues, err := ghClient.GetClosedIssuesWithLabels(repository, []string{board})
	if err != nil {
		log.Warn("failed to fetch closed issues for hierarchy mapping",
			"error", err,
			"board", board)
	} else {
		allIssues = append(allIssues, closedIssues...)
		log.Debug("including closed issues in hierarchy mapping",
			"open_count", len(issues),
			"closed_count", len(closedIssues),
			"total_count", len(allIssues))
	}

	// Create a map of GitHub issue numbers to JIRA IDs including closed issues
	githubToJira := make(map[int]string)
	for _, issue := range allIssues {
		if jiraID := parseJiraIDFromTitle(issue.Title); jiraID != "" {
			githubToJira[issue.Number] = jiraID
			log.Debug("mapped github issue to jira",
				"github_number", issue.Number,
				"jira_id", jiraID)
		}
	}

	// Process each feature to establish hierarchies
	for _, issue := range issues {
		if !hasLabel(issue.Labels, "feature") {
			continue
		}

		parentJiraID := parseJiraIDFromTitle(issue.Title)
		if parentJiraID == "" {
			continue
		}

		// Get child issue numbers from the description using configured domain
		childNums := parseChildIssues(issue.Description, cfg.GitHub.Domain)
		if len(childNums) == 0 {
			continue
		}

		log.Debug("found child issues in feature description",
			"parent_jira", parentJiraID,
			"child_count", len(childNums),
			"github_domain", cfg.GitHub.Domain)

		// Get existing links
		existingLinks, err := jiraClient.GetIssueLinks(parentJiraID)
		if err != nil {
			log.Error("failed to get existing links",
				"error", err,
				"parent", parentJiraID)
			continue
		}

		log.Debug("current JIRA links",
			"parent", parentJiraID,
			"existing_links", existingLinks)

		// Track which children should exist
		validChildren := make(map[string]bool)

		// Process each child issue number and build validChildren map
		for _, numStr := range childNums {
			num, err := strconv.Atoi(numStr)
			if err != nil {
				log.Error("invalid issue number",
					"number", numStr,
					"error", err)
				continue
			}

			// Get the JIRA ID for this GitHub issue number
			childJiraID, exists := githubToJira[num]
			if !exists {
				log.Debug("no JIRA ID found for GitHub issue",
					"github_number", num)
				continue
			}

			validChildren[childJiraID] = true

			// Only create the link if it doesn't already exist
			if !existingLinks[childJiraID] {
				log.Info("creating parent-child link",
					"parent", parentJiraID,
					"child", childJiraID)

				err := jiraClient.CreateParentChildLink(parentJiraID, childJiraID)
				if err != nil {
					log.Error("failed to create parent-child link",
						"error", err,
						"parent", parentJiraID,
						"child", childJiraID)
				} else {
					linksCreated++
				}
			} else {
				log.Debug("keeping existing link",
					"parent", parentJiraID,
					"child", childJiraID)
			}
		}

		log.Debug("valid children from GitHub",
			"parent", parentJiraID,
			"valid_children", validChildren)

		// Remove any existing links that are no longer valid
		for childID := range existingLinks {
			if !validChildren[childID] {
				log.Info("removing outdated parent-child link",
					"parent", parentJiraID,
					"child", childID,
					"reason", "not in GitHub issues section",
					"valid_children", validChildren)

				err := jiraClient.DeleteIssueLink(parentJiraID, childID)
				if err != nil {
					log.Error("failed to remove parent-child link",
						"error", err,
						"parent", parentJiraID,
						"child", childID)
				} else {
					linksRemoved++
				}
			}
		}
	}

	// Add summary log at the end
	log.Info("parent-child relationship synchronization complete",
		"board", board,
		"relationships_created", linksCreated,
		"relationships_removed", linksRemoved)

	return nil
}

// Helper function to check if a link already exists
func hasExistingLink(links map[string]bool, childID string) bool {
	return links[childID]
}

// Helper function to parse JIRA ID from title
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

func parseGitHubIssueLink(link string, gitHubDomain string) (string, int, error) {
	escapedDomain := regexp.QuoteMeta(gitHubDomain)
	pattern := fmt.Sprintf(`https://%s/([^/]+/[^/]+)/issues/(\d+)`, escapedDomain)
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(link)

	if len(matches) != 3 {
		return "", 0, fmt.Errorf("invalid GitHub issue link format: %s", link)
	}

	repo := matches[1]
	issueNum, err := strconv.Atoi(matches[2])
	if err != nil {
		return "", 0, fmt.Errorf("failed to parse issue number: %v", err)
	}

	return repo, issueNum, nil
}