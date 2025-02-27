package models

// GitHubIssue represents a GitHub issue
type GitHubIssue struct {
	Number      int
	Title       string
	Description string
	Labels      []string
	Type        string // Can be "story", "feature", or empty
}

// JiraTicket represents a JIRA ticket
type JiraTicket struct {
	ID            string
	Key           string
	Title         string
	Description   string
	Type          string // Can be "Story", "Feature", or other JIRA types
	CreatedByGlue bool
}

// TrelloCard represents a Trello card
type TrelloCard struct {
	ID            string
	Name          string
	Description   string
	Labels        []string
	CreatedByGlue bool
}
