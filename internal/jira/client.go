// Package jira provides functionality for interacting with the JIRA API.
package jira

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	jira "github.com/andygrunwald/go-jira"
	"github.com/danielolaszy/glue/internal/logging"
	"github.com/danielolaszy/glue/pkg/models"
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

// NewClient creates a new JIRA API client instance using credentials from environment
// variables. It tests the connection by retrieving the current user. It returns a
// configured client or an error if authentication fails.
func NewClient() (*Client, error) {
	// Get JIRA credentials from environment variables
	jiraURL := os.Getenv("JIRA_URL")
	jiraUsername := os.Getenv("JIRA_USERNAME")
	jiraToken := os.Getenv("JIRA_TOKEN")

	logging.Info("jira configuration",
		"base_url", jiraURL,
		"username", jiraUsername,
		"token_length", len(jiraToken))

	// Validate credentials
	var missingVars []string
	if jiraURL == "" {
		missingVars = append(missingVars, "JIRA_URL")
	}
	if jiraUsername == "" {
		missingVars = append(missingVars, "JIRA_USERNAME")
	}
	if jiraToken == "" {
		missingVars = append(missingVars, "JIRA_TOKEN")
	}

	if len(missingVars) > 0 {
		return nil, fmt.Errorf("missing required environment variables: %v", missingVars)
	}

	// Create transport for basic auth
	tp := jira.BasicAuthTransport{
		Username: jiraUsername,
		Password: jiraToken,
	}

	// Create JIRA client
	client, err := jira.NewClient(tp.Client(), jiraURL)
	if err != nil {
		logging.Error("failed to create jira client", "error", err)
		return nil, fmt.Errorf("error creating jira client: %v", err)
	}

	// Test the connection
	myself, resp, err := client.User.GetSelf()
	if err != nil {
		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
		}
		logging.Error("failed to test jira connection", 
			"error", err,
			"status_code", statusCode)
		return nil, fmt.Errorf("error testing jira connection: %v", err)
	}

	logging.Info("jira authentication successful", 
		"username", myself.DisplayName)

	return &Client{
		client:         client,
		issueTypeCache: make(map[string]map[string]string),
	}, nil
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

	// Try to get Feature Name field ID, but don't error if not found
	featureNameField, fieldType, err := c.getCustomField("Feature Name")
	if err != nil {
		logging.Debug("Feature Name field not found, continuing without it", 
			"error", err)
	} else {
		// Only add Feature Name field if we found the field ID
		issueFields.Unknowns = map[string]interface{}{
			featureNameField: issue.Title,
		}
		logging.Debug("adding Feature Name field", 
			"field_id", featureNameField,
			"field_type", fieldType,
			"value", issue.Title)
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
// It returns the link ID if found, empty string if not found, and an error if the check fails.
func (c *Client) GetIssueLinkID(parentKey, childKey string) (string, error) {
	logging.Debug("finding issue link ID in JIRA",
		"parent", parentKey,
		"child", childKey)

	// Check if the client is initialized
	if c.client == nil {
		return "", fmt.Errorf("jira client not initialized")
	}

	// Get the child issue with its links
	childIssue, resp, err := c.client.Issue.Get(childKey, nil)
	if err != nil {
		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
		}
		return "", fmt.Errorf("failed to get child issue: %v (status: %d)", err, statusCode)
	}

	// Check if there are any links
	if childIssue.Fields.IssueLinks == nil || len(childIssue.Fields.IssueLinks) == 0 {
		return "", nil
	}

	// Check each link to see if it connects to the parent
	for _, link := range childIssue.Fields.IssueLinks {
		// Check outward links (where the child is the inward issue)
		if link.OutwardIssue != nil && link.OutwardIssue.Key == parentKey {
			return link.ID, nil
		}

		// Check inward links (where the child is the outward issue)
		if link.InwardIssue != nil && link.InwardIssue.Key == parentKey {
			return link.ID, nil
		}
	}

	return "", nil
}

// DeleteIssueLink removes a link between two JIRA issues.
// It returns an error if the deletion fails.
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
		return nil // No link to delete
	}

	// Create the request to delete the link
	req, err := c.client.NewRequest(http.MethodDelete, "rest/api/2/issueLink/"+linkID, nil)
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
		return fmt.Errorf("failed to delete issue link: %v (status: %d)", err, statusCode)
	}

	logging.Info("successfully removed issue link",
		"parent", parentKey,
		"child", childKey)

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
