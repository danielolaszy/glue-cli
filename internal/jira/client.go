// Package jira provides functionality for interacting with the JIRA API.
package jira

import (
	"fmt"
	"io"
	"os"
	"strings"

	jira "github.com/andygrunwald/go-jira"
	"github.com/danielolaszy/glue/pkg/models"
	"github.com/danielolaszy/glue/internal/logging"

)

// Client handles interactions with the JIRA API.
type Client struct {
	client *jira.Client
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
	
	// Create transport for authentication
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
	
	// Verify client is properly initialized
	if client == nil {
		return nil, fmt.Errorf("jira client is nil after initialization")
	}
	
	if client.Issue == nil {
		return nil, fmt.Errorf("jira client Issue service is nil")
	}
	
	// Test the connection by getting the current user
	myself, resp, err := client.User.GetSelf()
	if err != nil {
		logging.Error("failed to test jira connection", "error", err, "status_code", resp.StatusCode)
		return nil, fmt.Errorf("error testing jira connection: %v", err)
	}
	logging.Info("jira authentication successful", "username", myself.EmailAddress)
	
	return &Client{
		client: client,
		issueTypeCache: make(map[string]map[string]string),
	}, nil
}

// CreateTicket creates a new JIRA ticket from a GitHub issue. It verifies the project
// exists, determines the appropriate issue type, and creates the ticket with a formatted
// description. It returns the ID of the created JIRA ticket (e.g., "ABC-123") or an
// error if creation fails.
func (c *Client) CreateTicket(projectKey string, issue models.GitHubIssue, issueType string) (string, error) {
	if c.client == nil {
		return "", fmt.Errorf("jira client not initialized")
	}

	// First, let's verify the project exists and get available issue types
	project, resp, err := c.client.Project.Get(projectKey)
	if err != nil {
		logging.Error("failed to get jira project", 
			"project_key", projectKey, 
			"error", err, 
			"status_code", resp.StatusCode)
		return "", fmt.Errorf("failed to get jira project '%s': %v (status: %d)", 
			projectKey, err, resp.StatusCode)
	}
	
	// Debug: Print available issue types
	logging.Debug("available issue types", "project", projectKey)
	validIssueType := false
	var availableTypes []string
	
	for _, issueTypeObj := range project.IssueTypes {
		availableTypes = append(availableTypes, issueTypeObj.Name)
		logging.Debug("issue type", "name", issueTypeObj.Name, "id", issueTypeObj.ID)
		if issueTypeObj.Name == issueType {
			validIssueType = true
		}
	}
	
	// If the requested issue type doesn't exist, use the first available type
	if !validIssueType {
		if len(availableTypes) > 0 {
			logging.Warn("issue type not found, using alternative", 
				"requested_type", issueType, 
				"using_type", availableTypes[0])
			issueType = availableTypes[0]
		} else {
			logging.Error("no issue types available in project", "project_key", projectKey)
			return "", fmt.Errorf("no issue types available in project '%s'", projectKey)
		}
	}

	// Prepare issue fields
	description := fmt.Sprintf("%s\n\n----\nCreated by glue from GitHub issue #%d",
		issue.Description, issue.Number)

	logging.Info("creating jira ticket", 
		"project", projectKey, 
		"type", issueType, 
		"title", issue.Title)
	
	issueFields := &jira.IssueFields{
		Project: jira.Project{
			Key: projectKey,
		},
		Summary:     issue.Title,
		Description: description,
		Type: jira.IssueType{
			Name: issueType,
		},
	}

	// Create the issue
	jiraIssue := &jira.Issue{
		Fields: issueFields,
	}

	logging.Debug("sending request to jira api")
	
	newIssue, resp, err := c.client.Issue.Create(jiraIssue)
	if err != nil {
		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
			
			// Try to get more details about the error
			body, readErr := io.ReadAll(resp.Body)
			if readErr == nil {
				logging.Error("failed to create jira ticket", 
					"error", err, 
					"status_code", statusCode, 
					"response", string(body))
				return "", fmt.Errorf("failed to create jira ticket: %v (status: %d, response: %s)", 
					err, statusCode, string(body))
			}
		}
		logging.Error("failed to create jira ticket", "error", err, "status_code", statusCode)
		return "", fmt.Errorf("failed to create jira ticket: %v (status: %d)", err, statusCode)
	}

	if newIssue == nil {
		logging.Error("jira api returned nil issue")
		return "", fmt.Errorf("jira api returned nil issue")
	}

	logging.Info("created jira ticket", "key", newIssue.Key)
	return newIssue.Key, nil
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

// AddIssueType creates a new issue type in JIRA. It handles both standard and
// subtask types. It returns the created issue type or an error if creation fails.
func (c *Client) AddIssueType(typeName string, isSubTask bool) (*jira.IssueType, error) {
	if c.client == nil {
		return nil, fmt.Errorf("jira client not initialized")
	}

	logging.Info("creating new issue type", "type", typeName, "is_subtask", isSubTask)

	// Prepare the request payload
	typeCategory := "standard"
	if isSubTask {
		typeCategory = "subtask"
	}

	newIssueType := struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Type        string `json:"type"`
	}{
		Name:        typeName,
		Description: fmt.Sprintf("Issue type for %s", typeName),
		Type:        typeCategory,
	}

	// Create the request
	req, err := c.client.NewRequest("POST", "rest/api/2/issuetype", newIssueType)
	if err != nil {
		logging.Error("failed to create request for new issue type", 
			"type", typeName, 
			"error", err)
		return nil, fmt.Errorf("failed to create request for new issue type: %v", err)
	}

	// Send the request
	issueTypeResponse := new(jira.IssueType)
	resp, err := c.client.Do(req, issueTypeResponse)
	if err != nil {
		statusCode := 0
		errorDetails := ""
		
		if resp != nil {
			statusCode = resp.StatusCode
			
			// Try to get more details about the error
			body, readErr := io.ReadAll(resp.Body)
			if readErr == nil {
				errorDetails = string(body)
			}
		}
		
		logging.Error("failed to create issue type", 
			"type", typeName, 
			"error", err, 
			"status_code", statusCode,
			"response", errorDetails)
			
		if errorDetails != "" {
			return nil, fmt.Errorf("failed to create issue type: %v (status: %d, response: %s)", 
				err, statusCode, errorDetails)
		}
		
		return nil, fmt.Errorf("failed to create issue type: %v", err)
	}

	logging.Info("successfully created issue type", 
		"type", typeName, 
		"type_id", issueTypeResponse.ID)

	return issueTypeResponse, nil
}

// CanCreateIssueTypes checks if the current user has permission to create issue types
// by attempting to access the project metadata. It returns whether the user likely has
// permission and any error that occurred during checking.
func (c *Client) CanCreateIssueTypes(projectKey string) (bool, error) {
	if c.client == nil {
		return false, fmt.Errorf("jira client not initialized")
	}

	logging.Debug("checking if user can create issue types", "project", projectKey)

	// Try to get metadata, which requires certain permissions
	metadata, resp, err := c.client.Issue.GetCreateMeta(projectKey)
	if err != nil {
		logging.Error("failed to get jira create metadata", 
			"project", projectKey, 
			"error", err, 
			"status_code", resp.StatusCode)
		return false, fmt.Errorf("failed to get jira create metadata: %v", err)
	}

	// Check if we have access to the project
	for _, project := range metadata.Projects {
		if project.Key == projectKey {
			// If we can get the metadata, we might have permission
			logging.Debug("user has access to project metadata", "project", projectKey)
			return true, nil
		}
	}

	logging.Debug("user may not have permission to create issue types", "project", projectKey)
	return false, nil
}

// EnsureIssueTypeExists checks if an issue type exists in the JIRA project and
// creates it if it doesn't. It returns the type ID and any error that occurred.
func (c *Client) EnsureIssueTypeExists(projectKey, typeName string) (string, error) {
	if c.client == nil {
		return "", fmt.Errorf("jira client not initialized")
	}

	logging.Info("ensuring issue type exists", "project", projectKey, "type", typeName)

	// Check if the issue type already exists
	exists, typeID, err := c.IssueTypeExists(projectKey, typeName)
	if err != nil {
		return "", err
	}

	if exists {
		logging.Info("issue type already exists", "project", projectKey, "type", typeName, "type_id", typeID)
		return typeID, nil
	}

	// Check if we have permission to create issue types
	canCreate, err := c.CanCreateIssueTypes(projectKey)
	if err != nil {
		logging.Debug("failed to check permissions for creating issue types", 
			"project", projectKey, 
			"error", err)
		// Continue anyway, we might still be able to find the type
	}

	if !canCreate {
		logging.Debug("user may not have permission to create issue types", 
			"project", projectKey)
		// Continue anyway, the type might already exist
	}

	// Create the issue type
	isSubTask := false // Assume standard issue type
	newType, err := c.AddIssueType(typeName, isSubTask)
	if err != nil {
		return "", err
	}

	return newType.ID, nil
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

// CreateTicketWithTypeID creates a new JIRA ticket with a specific issue type ID.
// It returns the ID of the created ticket or an error if creation fails.
func (c *Client) CreateTicketWithTypeID(projectKey string, issue models.GitHubIssue, issueTypeID string) (string, error) {
	if c.client == nil {
		return "", fmt.Errorf("jira client not initialized")
	}

	// Prepare issue fields
	description := fmt.Sprintf("%s\n\n----\nCreated by glue from GitHub issue #%d",
		issue.Description, issue.Number)

	logging.Info("creating jira ticket", 
		"project", projectKey, 
		"title", issue.Title, 
		"type_id", issueTypeID)
	
	issueFields := &jira.IssueFields{
		Project: jira.Project{
			Key: projectKey,
		},
		Summary:     issue.Title,
		Description: description,
		Type: jira.IssueType{
			ID: issueTypeID, // Use ID instead of name
		},
	}

	// Create the issue
	jiraIssue := &jira.Issue{
		Fields: issueFields,
	}

	logging.Debug("sending request to jira api")
	
	newIssue, resp, err := c.client.Issue.Create(jiraIssue)
	if err != nil {
		statusCode := 0
		if resp != nil {
			statusCode = resp.StatusCode
			
			// Try to get more details about the error
			body, readErr := io.ReadAll(resp.Body)
			if readErr == nil {
				logging.Error("failed to create jira ticket", 
					"error", err, 
					"status_code", statusCode, 
					"response", string(body))
				return "", fmt.Errorf("failed to create jira ticket: %v (status: %d, response: %s)", 
					err, statusCode, string(body))
			}
		}
		logging.Error("failed to create jira ticket", "error", err, "status_code", statusCode)
		return "", fmt.Errorf("failed to create jira ticket: %v (status: %d)", err, statusCode)
	}

	if newIssue == nil {
		logging.Error("jira api returned nil issue")
		return "", fmt.Errorf("jira api returned nil issue")
	}

	logging.Info("created jira ticket", "key", newIssue.Key)
	return newIssue.Key, nil
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
	req, err := c.client.NewRequest("POST", "rest/api/2/issueLink", linkData)
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
