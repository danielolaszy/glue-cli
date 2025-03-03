// Package models defines data structures shared across the application.
package models

import (
	"time"
)

// GitHubIssue represents a GitHub issue with its essential fields
type GitHubIssue struct {
	// Number is the issue number in GitHub (e.g., 42)
	Number int

	// Title is the issue's title or summary
	Title string

	// Description is the full body text of the issue
	Description string

	// State is the current state of the issue
	State string

	// CreatedAt is the timestamp when the issue was created
	CreatedAt time.Time

	// UpdatedAt is the timestamp when the issue was last updated
	UpdatedAt time.Time

	// ClosedAt is the timestamp when the issue was closed
	ClosedAt *time.Time

	// Labels is a slice of label names attached to the issue
	Labels []string
}

// JiraTicket represents a JIRA ticket with its key properties.
type JiraTicket struct {
	// ID is the numeric part of the JIRA ticket ID (e.g., 123 from "ABC-123")
	ID string

	// Key is the full JIRA ticket identifier (e.g., "ABC-123")
	Key string

	// Title is the ticket's summary field
	Title string

	// Description is the full body text of the ticket
	Description string

	// Type is the JIRA issue type (e.g., "Story", "Feature", "Task")
	Type string

	// CreatedByGlue indicates whether this ticket was created by our tool
	CreatedByGlue bool
}
