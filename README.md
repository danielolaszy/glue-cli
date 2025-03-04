# Glue

Glue is a CLI tool that synchronizes GitHub issues with JIRA tickets, maintaining relationships between features and their child stories. It helps teams that use both GitHub for development and JIRA for project management keep their work items synchronized.

## Why Glue?

Many development teams prefer GitHub's streamlined interface and integrated development workflows. However, organizations often require JIRA for:

- **Software Capitalization**: Tracking development costs and capitalizing software development expenses for financial compliance
- **Budget Tracking**: Managing development costs and resource allocation at an enterprise level
- **Compliance**: Maintaining accurate records of development efforts for financial reporting and auditing

Glue lets development teams continue working exclusively in GitHub while automatically keeping JIRA in sync. Instead of forcing developers to manually maintain two systems, Glue:

- Allows teams to create and manage all issues in GitHub
- Automatically creates and updates corresponding JIRA tickets
- Maintains feature/story relationships across both systems
- Syncs status changes (like closing issues) from GitHub to JIRA

This means development teams can stay focused on GitHub's development experience while satisfying organizational requirements for JIRA-based tracking and reporting.

## Features

- Creates JIRA tickets from GitHub issues
- Maintains parent-child relationships between features and stories
- Synchronizes issue status (closed GitHub issues are reflected in JIRA)
- Supports multiple JIRA boards/projects
- Preserves existing relationships and only updates when necessary
- Detailed logging for troubleshooting

## Prerequisites

### GitHub Repository Setup

Your GitHub repository needs the following labels:

- `feature`: Applied to issues that should be created as Features in JIRA
- `story`: Applied to issues that should be created as Stories in JIRA
- The JIRA project key(s) (e.g., `PROJ`, `TESTGCP`) as labels to indicate which JIRA project the issue belongs to

### JIRA Project Setup

Your JIRA project needs these issue types configured:

- `Feature`: For parent/epic-level issues
- `Story`: For child/task-level issues

If the Story type isn't available, issues will default to the Feature type.

### Authentication

The tool requires authentication tokens for both GitHub and JIRA:

1. GitHub Personal Access Token with `repo` scope
2. JIRA API Token

These should be configured in your environment:

```bash
export GITHUB_TOKEN=your_github_token
export JIRA_TOKEN=your_jira_token
export JIRA_USERNAME=your_jira_email
export JIRA_URL=https://your-domain.atlassian.net
```

## Installation

```bash
go install github.com/yourusername/glue@latest
```

## Usage

### Basic Command

```bash
glue jira -r owner/repository -b PROJ1 [-b PROJ2 ...]
```

### Command Line Flags

- `-r, --repository`: GitHub repository in the format `owner/repository` (required)
- `-b, --board`: JIRA project key(s). Can be specified multiple times for multiple projects

### Debug Logging

Debug logging is controlled via the `LOG_LEVEL` environment variable:

```bash
export LOG_LEVEL=debug
glue jira -r owner/repo -b PROJ
```

### Examples

Sync with a single JIRA project:
```bash
glue jira -r myorg/myrepo -b PROJ
```

Sync with multiple JIRA projects:
```bash
glue jira -r myorg/myrepo -b PROJ1 -b PROJ2
```

## How It Works

### Issue Creation

1. The tool fetches all GitHub issues labeled with the specified JIRA project key(s)
2. For each issue:
   - If labeled with `feature`, creates a JIRA Feature
   - If labeled with `story`, creates a JIRA Story
   - Otherwise, defaults to creating a Story
3. Updates the GitHub issue title with the JIRA ID: `[PROJ-123] Original Title`

### Parent-Child Relationships

Features can specify their child issues in the description using a `## Issues` section:

```markdown
This is a feature description

## Issues
- https://github.com/owner/repo/issues/1
- https://github.com/owner/repo/issues/2
```

The tool will:
1. Parse the Issues section
2. Create "relates to" relationships in JIRA between the feature and its stories
3. Maintain these relationships over time, adding/removing as the Issues section changes

### Status Synchronization

When GitHub issues are closed:
1. The tool identifies corresponding JIRA tickets
2. Moves them to "Done" status if not already closed
3. Maintains parent-child relationships even for closed issues

## Best Practices

1. **Issue Organization**:
   - Use the `feature` label for larger work items that will have child stories
   - Use the `story` label for individual tasks
   - Always include the JIRA project label (e.g., `PROJ`)

2. **Feature Management**:
   - Maintain the `## Issues` section in feature descriptions
   - List all related stories as GitHub issue links
   - Keep the list updated as stories are added/removed

3. **Synchronization**:
   - Run the tool regularly (e.g., via CI/CD) to keep systems in sync
   - Use debug logging when troubleshooting: `LOG_LEVEL=debug`


### Debug Logging

Enable detailed logging for troubleshooting:
```bash
LOG_LEVEL=debug glue jira -r owner/repo -b PROJ
```

### Additional Features Not Mentioned in Current README

1. **Fix Version Support**
The code automatically assigns JIRA tickets to the current PI (Program Increment) version if available.

2. **Feature Name Field**
The code attempts to set a custom "Feature Name" field in JIRA if it exists.

3. **Relationship Types**
The code uses "Relates" type links in JIRA for parent-child relationships.

4. **Retry Logic**
Both GitHub and JIRA clients implement retry logic for authentication and API calls.

## Configuration

The application is configured via environment variables:

### GitHub Configuration

- `GITHUB_DOMAIN` - The GitHub domain to use. Defaults to `github.example.com` (GitHub Enterprise).
  - Use `github.com` for public GitHub
  - For GitHub Enterprise, specify your custom domain (e.g., `github.mycompany.com`)
- `GITHUB_TOKEN` - GitHub personal access token with appropriate permissions (required)

### JIRA Configuration

- `JIRA_URL` - The base URL of your JIRA instance (required)
- `JIRA_USERNAME` - JIRA username for authentication (required)
- `JIRA_TOKEN` - JIRA API token for authentication (required)
