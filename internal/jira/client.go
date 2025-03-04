// Package jira provides functionality for interacting with the JIRA API.
package jira

import (
	"fmt"
	"net/http"
	"errors"
	"strings"
	"time"

	jira "github.com/andygrunwald/go-jira"
	"github.com/danielolaszy/glue/internal/logging"
	"github.com/danielolaszy/glue/pkg/models"
	"github.com/danielolaszy/glue/internal/config"
)

// Client handles interactions with the JIRA API.
type Client struct {
	client   *jira.Client
	BaseURL  string
	Username string
	Token    string
	// Cache for issue types by project key
	issueTypeCache map[string]map[string]string // projectKey -> typeName -> typeID
}

// NewClient creates a new JIRA client with the provided configuration.
func NewClient() (*Client, error) {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}
	
	// Log the configuration
	logging.Info("jira configuration",
		"base_url", cfg.Jira.BaseURL,
		"username", cfg.Jira.Username,
		"token_length", len(cfg.Jira.Token))
	
	// Validate required configuration
	if cfg.Jira.BaseURL == "" || cfg.Jira.Username == "" || cfg.Jira.Token == "" {
		return nil, errors.New("missing required JIRA configuration (JIRA_URL, JIRA_USERNAME, JIRA_TOKEN)")
	}
	
	// Create transport for authentication
	tp := jira.BasicAuthTransport{
		Username: cfg.Jira.Username,
		Password: cfg.Jira.Token,
	}
	
	// Create JIRA client
	jiraClient, err := jira.NewClient(tp.Client(), cfg.Jira.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create JIRA client: %w", err)
	}
	
	// Create client wrapper
	client := &Client{
		client: jiraClient,
		issueTypeCache: make(map[string]map[string]string),
	}
	
	// Test authentication with retries
	maxRetries := 3
	var authError error
	
	for attempt := 1; attempt <= maxRetries; attempt++ {
		_, _, err := jiraClient.User.GetSelf()
		if err == nil {
			// Authentication successful
			logging.Info("jira authentication successful")
			return client, nil
		}
		
		authError = err  // Store the last error
		
		logging.Warn("jira authentication attempt failed, retrying...",
			"attempt", attempt,
			"error", err)
		
		// Only retry if this is not the last attempt
		if attempt < maxRetries {
			time.Sleep(time.Duration(attempt) * time.Second)
		} else {
			// Log final error
			logging.Error("all jira authentication attempts failed",
				"attempts", maxRetries,
				"final_error", err)
		}
	}
	
	// If authentication failed, return error
	return nil, fmt.Errorf("failed to authenticate with JIRA: %w", authError)
}

// GetTotalTickets returns the total number of tickets in a JIRA project by executing
// a JQL search. It returns the count or an error if the query fails.
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

// IssueTypeExists checks if an issue type exists in the JIRA project. It returns
// whether the type exists, the type ID if found, and any error that occurred.
func (c *Client) IssueTypeExists(projectKey, typeName string) (bool, string, error) {
	if c.client == nil {
		return false, "", fmt.Errorf("jira client not initialized")
	}

	logging.Debug("checking if issue type exists", "project", projectKey, "type", typeName)

	// Get the project to see available issue types
	project, resp, err := c.client.Project.Get(projectKey)
	if err != nil {
		logging.Error("failed to get jira project",
			"project", projectKey,
			"error", err,
			"status_code", resp.StatusCode)
		return false, "", fmt.Errorf("failed to get jira project '%s': %v", projectKey, err)
	}

	// Check if the issue type already exists
	for _, issueType := range project.IssueTypes {
		if strings.EqualFold(issueType.Name, typeName) {
			logging.Debug("issue type found",
				"project", projectKey,
				"type", typeName,
				"type_id", issueType.ID)
			return true, issueType.ID, nil
		}
	}

	logging.Debug("issue type not found", "project", projectKey, "type", typeName)
	return false, "", nil
}

// GetIssueTypeID retrieves the ID of a specific issue type from a JIRA project.
// It checks the cache first and loads issue types for the project if necessary.
// It returns the type ID or an error if the type cannot be found.
func (c *Client) GetIssueTypeID(projectKey, typeName string) (string, error) {
	typeName = strings.ToLower(typeName)
	logging.Debug("retrieving issue type id", "project", projectKey, "type", typeName)

	// Check if we have cached issue types for this project
	if projectTypes, exists := c.issueTypeCache[projectKey]; exists {
		// Check if the requested type exists in the cache
		if typeID, exists := projectTypes[typeName]; exists {
			logging.Info("found issue type in cache", "name", typeName, "id", typeID)
			return typeID, nil
		}
	} else {
		// Load issue types for the project
		err := c.LoadIssueTypes(projectKey)
		if err != nil {
			return "", err
		}

		// Now check the cache again
		if typeID, exists := c.issueTypeCache[projectKey][typeName]; exists {
			logging.Info("found issue type", "name", typeName, "id", typeID)
			return typeID, nil
		}
	}

	// If we reach here, the issue type doesn't exist in the project
	return "", fmt.Errorf("issue type '%s' not found in project '%s'", typeName, projectKey)
}

// getCustomField retrieves the custom field ID by its name.
// It returns the field ID, field type, and any error that occurred.
func (c *Client) getCustomField(name string) (string, string, error) {
	if c.client == nil {
		return "", "", fmt.Errorf("jira client not initialized")
	}

	logging.Debug("getting custom field ID", "name", name)

	// Get all fields
	req, err := c.client.NewRequest("GET", "rest/api/2/field", nil)
	if err != nil {
		return "", "", fmt.Errorf("failed to create request for getting fields: %v", err)
	}

	var fields []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Schema struct {
			Type   string `json:"type"`
			Custom string `json:"custom,omitempty"`
		} `json:"schema"`
	}

	resp, err := c.client.Do(req, &fields)
	if err != nil {
		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
		}
		return "", "", fmt.Errorf("failed to get fields: %v (status: %d)", err, statusCode)
	}

	// Find the field with matching name
	for _, field := range fields {
		if field.Name == name {
			logging.Debug("found custom field",
				"name", name,
				"id", field.ID,
				"type", field.Schema.Type,
				"custom", field.Schema.Custom)
			return field.ID, field.Schema.Type, nil
		}
	}

	return "", "", fmt.Errorf("custom field '%s' not found", name)
}

// CreateTicketWithTypeID creates a new JIRA ticket with the specified type ID.
func (c *Client) CreateTicketWithTypeID(projectKey string, issue models.GitHubIssue, issueTypeID string) (string, error) {
	if c.client == nil {
		return "", fmt.Errorf("jira client not initialized")
	}

	// Get the default fix version for the project
	fixVersion, err := c.GetDefaultFixVersion(projectKey)
	if err != nil {
		logging.Error("failed to get default fix version", "error", err)
		// Continue without fix version
	}

	logging.Info("creating jira ticket",
		"project", projectKey,
		"title", issue.Title,
		"type_id", issueTypeID)

	// Create the basic issue fields
	issueFields := &jira.IssueFields{
		Project: jira.Project{
			Key: projectKey,
		},
		Summary:     issue.Title,
		Description: issue.Description,
		Type: jira.IssueType{
			ID: issueTypeID,
		},
	}

	// Add fix version if available
	if fixVersion != nil {
		issueFields.FixVersions = []*jira.FixVersion{fixVersion}
		logging.Info("adding fix version to ticket",
			"version", fixVersion,
			"version_id", fixVersion.ID)
	}

	// Initialize Unknowns map for custom fields
	issueFields.Unknowns = map[string]interface{}{}

	// Try to get Feature Name field ID, but don't error if not found
	featureNameField, fieldType, err := c.getCustomField("Feature Name")
	if err != nil {
		logging.Debug("Feature Name field not found, continuing without it", 
			"error", err)
	} else {
		// Add Feature Name field if we found the field ID
		issueFields.Unknowns[featureNameField] = issue.Title
		logging.Debug("adding Feature Name field", 
			"field_id", featureNameField,
			"field_type", fieldType,
			"value", issue.Title)
	}

	// Try to get Primary Feature Work Type field ID, but don't error if not found
	workTypeField, workTypeFieldType, err := c.getCustomField("Primary Feature Work Type :")
	if err != nil {
		logging.Debug("Primary Feature Work Type field not found, continuing without it", 
			"error", err)
	} else {
		// Add Primary Feature Work Type field with hardcoded value
		const workTypeValue = "Other Non-Application Development activities"
		issueFields.Unknowns[workTypeField] = workTypeValue
		logging.Debug("adding Primary Feature Work Type field", 
			"field_id", workTypeField,
			"field_type", workTypeFieldType,
			"value", workTypeValue)
	}

	// Create the issue
	newIssue := &jira.Issue{
		Fields: issueFields,
	}

	createdIssue, resp, err := c.client.Issue.Create(newIssue)
	if err != nil {
		logging.Error("failed to create jira ticket",
			"error", err,
			"status_code", resp.StatusCode)
		return "", fmt.Errorf("failed to create jira ticket: %v", err)
	}

	logging.Info("created jira ticket", "key", createdIssue.Key)
	return createdIssue.Key, nil
}

// LoadIssueTypes loads all issue types for a project into the cache to avoid
// repeated API calls. It returns an error if loading fails.
func (c *Client) LoadIssueTypes(projectKey string) error {
	logging.Debug("loading all issue types for project", "project", projectKey)

	// Create the cache entry for this project if it doesn't exist
	if _, exists := c.issueTypeCache[projectKey]; !exists {
		c.issueTypeCache[projectKey] = make(map[string]string)
	}

	// Get all issue types for the project
	project, _, err := c.client.Project.Get(projectKey)
	if err != nil {
		logging.Error("failed to get project", "project", projectKey, "error", err)
		return err
	}

	logging.Debug("available issue types in project", "project", projectKey)
	for _, issueType := range project.IssueTypes {
		typeName := strings.ToLower(issueType.Name)
		typeID := issueType.ID

		// Cache the issue type
		c.issueTypeCache[projectKey][typeName] = typeID
		logging.Debug("cached issue type", "name", issueType.Name, "id", typeID)
	}

	return nil
}

// CreateParentChildLink establishes a relationship between JIRA tickets using
// the "Relates" link type. It returns an error if the linking operation fails.
func (c *Client) CreateParentChildLink(parentKey, childKey string) error {
	logging.Info("creating parent-child relationship in JIRA",
		"parent", parentKey,
		"child", childKey)

	// Check if the client is initialized
	if c.client == nil {
		return fmt.Errorf("jira client not initialized")
	}

	// Use issue linking API instead of parent field
	linkData := map[string]interface{}{
		"type": map[string]string{
			"name": "Relates", // Or another relationship type like "Blocks" or a custom one
		},
		"inwardIssue": map[string]string{
			"key": childKey,
		},
		"outwardIssue": map[string]string{
			"key": parentKey,
		},
	}

	// Create the request
	req, err := c.client.NewRequest(http.MethodPost, "rest/api/2/issueLink", linkData)
	if err != nil {
		return fmt.Errorf("failed to create request for linking issues: %v", err)
	}

	// Send the request
	resp, err := c.client.Do(req, nil)
	if err != nil {
		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
		}
		return fmt.Errorf("failed to link issues: %v (status: %d)", err, statusCode)
	}

	return nil
}

// CheckParentChildLinkExists checks if a parent-child link already exists in JIRA.
// It returns true if the link exists, false if it doesn't, and an error if the check fails.
func (c *Client) CheckParentChildLinkExists(parentKey, childKey string) (bool, error) {
	logging.Debug("checking if parent-child link exists in JIRA",
		"parent", parentKey,
		"child", childKey)

	// Check if the client is initialized
	if c.client == nil {
		return false, fmt.Errorf("jira client not initialized")
	}

	// Get the child issue with its links
	childIssue, resp, err := c.client.Issue.Get(childKey, nil)
	if err != nil {
		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
		}
		return false, fmt.Errorf("failed to get child issue: %v (status: %d)", err, statusCode)
	}

	// Check if there are any links
	if childIssue.Fields.IssueLinks == nil || len(childIssue.Fields.IssueLinks) == 0 {
		return false, nil
	}

	// Check each link to see if it connects to the parent
	for _, link := range childIssue.Fields.IssueLinks {
		// Check outward links (where the child is the inward issue)
		if link.OutwardIssue != nil && link.OutwardIssue.Key == parentKey {
			return true, nil
		}

		// Check inward links (where the child is the outward issue)
		if link.InwardIssue != nil && link.InwardIssue.Key == parentKey {
			return true, nil
		}
	}

	return false, nil
}

// GetIssueLinkID retrieves the ID of the link between two JIRA issues.
// It checks both the parent and child issues for links connecting them,
// and returns the link ID if found or an error if the retrieval fails.
func (c *Client) GetIssueLinkID(parentKey, childKey string) (string, error) {
	logging.Debug("finding issue link ID in JIRA",
		"parent", parentKey,
		"child", childKey)

	// Get both issues to check links from both sides
	parentIssue, _, err := c.client.Issue.Get(parentKey, &jira.GetQueryOptions{
		Expand: "issuelinks",
	})
	if err != nil {
		return "", fmt.Errorf("failed to get parent issue: %v", err)
	}

	// Log all links on parent issue
	for _, link := range parentIssue.Fields.IssueLinks {
		outwardKey := ""
		if link.OutwardIssue != nil {
			outwardKey = link.OutwardIssue.Key
		}
		inwardKey := ""
		if link.InwardIssue != nil {
			inwardKey = link.InwardIssue.Key
		}
		
		logging.Debug("examining parent link",
			"link_id", link.ID,
			"type", link.Type.Name,
			"outward_issue", link.OutwardIssue != nil,
			"inward_issue", link.InwardIssue != nil,
			"outward_key", outwardKey,
			"inward_key", inwardKey)
	}

	// Get child issue as well
	childIssue, _, err := c.client.Issue.Get(childKey, &jira.GetQueryOptions{
		Expand: "issuelinks",
	})
	if err != nil {
		return "", fmt.Errorf("failed to get child issue: %v", err)
	}

	// Log all links on child issue
	for _, link := range childIssue.Fields.IssueLinks {
		outwardKey := ""
		if link.OutwardIssue != nil {
			outwardKey = link.OutwardIssue.Key
		}
		inwardKey := ""
		if link.InwardIssue != nil {
			inwardKey = link.InwardIssue.Key
		}

		logging.Debug("examining child link",
			"link_id", link.ID,
			"type", link.Type.Name,
			"outward_issue", link.OutwardIssue != nil,
			"inward_issue", link.InwardIssue != nil,
			"outward_key", outwardKey,
			"inward_key", inwardKey)

		// For "Relates" type links, check both directions
		if link.Type.Name == "Relates" {
			if (link.OutwardIssue != nil && link.OutwardIssue.Key == parentKey) ||
			   (link.InwardIssue != nil && link.InwardIssue.Key == parentKey) {
				logging.Debug("found matching link to remove",
					"link_id", link.ID,
					"parent", parentKey,
					"child", childKey)
				return link.ID, nil
			}
		}
	}

	logging.Debug("no matching link found",
		"parent", parentKey,
		"child", childKey)
	return "", nil
}

// DeleteIssueLink removes a link between two JIRA issues.
func (c *Client) DeleteIssueLink(parentKey, childKey string) error {
	logging.Info("removing parent-child relationship in JIRA",
		"parent", parentKey,
		"child", childKey)

	// Check if the client is initialized
	if c.client == nil {
		return fmt.Errorf("jira client not initialized")
	}

	// First, find the ID of the link
	linkID, err := c.GetIssueLinkID(parentKey, childKey)
	if err != nil {
		return fmt.Errorf("failed to find link ID: %v", err)
	}

	if linkID == "" {
		logging.Debug("no link found to delete",
			"parent", parentKey,
			"child", childKey)
		return nil
	}

	// Create the request to delete the link
	// Note: The API endpoint is /rest/api/2/issueLink/{linkId}
	req, err := c.client.NewRequest(http.MethodDelete, fmt.Sprintf("rest/api/2/issueLink/%s", linkID), nil)
	if err != nil {
		return fmt.Errorf("failed to create request for deleting issue link: %v", err)
	}

	// Send the request
	resp, err := c.client.Do(req, nil)
	if err != nil {
		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
		}
		logging.Error("failed to delete issue link",
			"error", err,
			"status_code", statusCode,
			"link_id", linkID)
		return fmt.Errorf("failed to delete issue link: %v (status: %d)", err, statusCode)
	}

	logging.Info("successfully removed issue link",
		"parent", parentKey,
		"child", childKey,
		"link_id", linkID)

	return nil
}

// GetLinkedIssues retrieves all issue keys that are linked to the specified parent issue.
// It returns a slice of child issue keys or an error if retrieval fails.
func (c *Client) GetLinkedIssues(parentKey string) ([]string, error) {
	logging.Debug("retrieving linked issues in JIRA",
		"parent", parentKey)

	// Check if the client is initialized
	if c.client == nil {
		return nil, fmt.Errorf("jira client not initialized")
	}

	// Get the parent issue with its links
	parentIssue, resp, err := c.client.Issue.Get(parentKey, nil)
	if err != nil {
		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
		}
		return nil, fmt.Errorf("failed to get parent issue: %v (status: %d)", err, statusCode)
	}

	// Check if there are any links
	if parentIssue.Fields.IssueLinks == nil || len(parentIssue.Fields.IssueLinks) == 0 {
		return []string{}, nil
	}

	// Collect all linked issue keys
	var linkedIssues []string
	for _, link := range parentIssue.Fields.IssueLinks {
		// Look for outward links (where the parent is the inward issue)
		if link.OutwardIssue != nil {
			linkedIssues = append(linkedIssues, link.OutwardIssue.Key)
		}

		// Look for inward links (where the parent is the outward issue)
		if link.InwardIssue != nil {
			linkedIssues = append(linkedIssues, link.InwardIssue.Key)
		}
	}

	return linkedIssues, nil
}

// CloseTicket transitions a JIRA ticket to the "Done" status.
// It returns an error if the operation fails.
func (c *Client) CloseTicket(ticketKey string) error {
	logging.Info("closing jira ticket", "ticket", ticketKey)

	// Check if the client is initialized
	if c.client == nil {
		return fmt.Errorf("jira client not initialized")
	}

	// Get available transitions for the ticket
	transitions, resp, err := c.client.Issue.GetTransitions(ticketKey)
	if err != nil {
		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
		}
		return fmt.Errorf("failed to get transitions for ticket %s: %v (status: %d)",
			ticketKey, err, statusCode)
	}

	// Look for a "Done" or "Closed" transition
	var transitionID string
	for _, t := range transitions {
		name := strings.ToLower(t.Name)
		if name == "done" || name == "close" || name == "closed" || name == "resolve" || name == "resolved" {
			transitionID = t.ID
			break
		}
	}

	if transitionID == "" {
		return fmt.Errorf("no 'done' or 'close' transition found for ticket %s", ticketKey)
	}

	// Execute the transition
	resp, err = c.client.Issue.DoTransition(ticketKey, transitionID)
	if err != nil {
		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
		}
		return fmt.Errorf("failed to close ticket %s: %v (status: %d)",
			ticketKey, err, statusCode)
	}

	logging.Info("successfully closed jira ticket", "ticket", ticketKey)
	return nil
}

// GetProjectVersions retrieves all versions for a JIRA project.
// It returns a slice of versions or an error if retrieval fails.
func (c *Client) GetProjectVersions(projectKey string) ([]jira.Version, error) {
	if c.client == nil {
		return nil, fmt.Errorf("jira client not initialized")
	}

	logging.Debug("retrieving project versions", "project", projectKey)

	// Get project to access versions
	project, resp, err := c.client.Project.Get(projectKey)
	if err != nil {
		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
		}
		logging.Error("failed to get project versions",
			"project", projectKey,
			"error", err,
			"status_code", statusCode)
		return nil, fmt.Errorf("failed to get project versions: %v (status: %d)", err, statusCode)
	}

	return project.Versions, nil
}

// GetDefaultFixVersion returns the current PI version for a project.
// It selects a version that is:
// 1. Not released
// 2. Not archived
// 3. Has the closest PI number to current (e.g., PI 25.1 instead of PI 25.5)
func (c *Client) GetDefaultFixVersion(projectKey string) (*jira.FixVersion, error) {
	logging.Debug("getting default fix version", "project", projectKey)

	versions, err := c.GetProjectVersions(projectKey)
	if err != nil {
		logging.Error("failed to get project versions", "error", err)
		return nil, err
	}

	logging.Debug("found project versions", "count", len(versions))

	type piVersion struct {
		major    int
		minor    int
		version  *jira.Version
		released bool
		archived bool
	}

	var currentPI *piVersion
	// Parse and store PI versions
	for i := range versions {
		version := &versions[i]
		
		// Skip released or archived versions
		released := version.Released != nil && *version.Released
		archived := version.Archived != nil && *version.Archived
		if released || archived {
			logging.Debug("skipping released/archived version",
				"name", version.Name,
				"released", released,
				"archived", archived)
			continue
		}

		// Try to parse PI version (e.g., "PI 25.1")
		var major, minor int
		_, err := fmt.Sscanf(version.Name, "PI %d.%d", &major, &minor)
		if err != nil {
			logging.Debug("skipping non-PI version", "name", version.Name)
			continue
		}

		pv := &piVersion{
			major:    major,
			minor:    minor,
			version:  version,
			released: released,
			archived: archived,
		}

		logging.Debug("found PI version",
			"name", version.Name,
			"major", major,
			"minor", minor)

		// If this is our first version or if it's a better match for current PI
		if currentPI == nil {
			currentPI = pv
			continue
		}

		// If same major version, prefer lower minor version as it's likely more current
		if pv.major == currentPI.major && pv.minor < currentPI.minor {
			currentPI = pv
			logging.Debug("found better PI version",
				"name", version.Name,
				"previous", currentPI.version.Name)
			continue
		}

		// If different major version, prefer the higher one as it's more current
		if pv.major > currentPI.major {
			currentPI = pv
			logging.Debug("found newer PI version",
				"name", version.Name,
				"previous", currentPI.version.Name)
		}
	}

	// Convert Version to FixVersion
	if currentPI != nil {
		released := false
		if currentPI.version.Released != nil {
			released = *currentPI.version.Released
		}
		archived := false
		if currentPI.version.Archived != nil {
			archived = *currentPI.version.Archived
		}
		releasedPtr := &released
		archivedPtr := &archived

		logging.Info("selected fix version",
			"name", currentPI.version.Name,
			"id", currentPI.version.ID,
			"major", currentPI.major,
			"minor", currentPI.minor,
			"released", released,
			"archived", archived)

		return &jira.FixVersion{
			ID:          currentPI.version.ID,
			Name:        currentPI.version.Name,
			Description: currentPI.version.Description,
			Released:    releasedPtr,
			Archived:    archivedPtr,
		}, nil
	}

	logging.Info("no suitable fix version found")
	return nil, nil
}

// GetChildIssues retrieves all subtask issues directly associated with a given parent issue.
// It takes a parentID string representing the JIRA issue key (e.g., "PROJECT-123") and returns
// a slice of child issue keys or an error if the retrieval fails.
func (c *Client) GetChildIssues(parentID string) ([]string, error) {
	issue, _, err := c.client.Issue.Get(parentID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get issue: %v", err)
	}

	var children []string
	// Check the subtasks field
	for _, subtask := range issue.Fields.Subtasks {
		children = append(children, subtask.Key)
	}

	return children, nil
}

// GetIssueLinks retrieves all issues linked to the specified JIRA issue, regardless of link type.
// It takes an issueID string representing the JIRA issue key (e.g., "PROJECT-123") and returns
// a map where keys are the linked issue keys and values are always true, or an error if the 
// retrieval fails. The map acts as a set of unique linked issue keys.
func (c *Client) GetIssueLinks(issueID string) (map[string]bool, error) {
	logging.Debug("getting issue links", "issue", issueID)
	
	issue, _, err := c.client.Issue.Get(issueID, &jira.GetQueryOptions{
		Expand: "issuelinks",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get issue: %v", err)
	}

	children := make(map[string]bool)
	for _, link := range issue.Fields.IssueLinks {
		// Log the link type for debugging
		logging.Debug("found link",
			"issue", issueID,
			"type", link.Type.Name,
			"outward", link.OutwardIssue != nil,
			"inward", link.InwardIssue != nil)

		// Check both inward and outward links
		if link.OutwardIssue != nil {
			children[link.OutwardIssue.Key] = true
		}
		if link.InwardIssue != nil {
			children[link.InwardIssue.Key] = true
		}
	}

	logging.Debug("found linked issues",
		"issue", issueID,
		"links", children)

	return children, nil
}

// GetTicketStatus retrieves the current status of a JIRA ticket.
// It takes an issueID string representing the JIRA issue key (e.g., "PROJECT-123") and returns
// the status name as a string (e.g., "In Progress", "Done") or an error if the retrieval fails.
func (c *Client) GetTicketStatus(issueID string) (string, error) {
	if c.client == nil {
		return "", fmt.Errorf("jira client not initialized")
	}

	logging.Debug("getting ticket status", "ticket", issueID)

	issue, _, err := c.client.Issue.Get(issueID, &jira.GetQueryOptions{
		Fields: "status",
	})
	if err != nil {
		return "", fmt.Errorf("failed to get issue status: %v", err)
	}

	if issue == nil || issue.Fields == nil || issue.Fields.Status == nil {
		return "", fmt.Errorf("invalid issue response")
	}

	logging.Debug("got ticket status",
		"ticket", issueID,
		"status", issue.Fields.Status.Name)

	return issue.Fields.Status.Name, nil
}
