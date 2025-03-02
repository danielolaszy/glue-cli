# Glue CLI

Glue is a command-line tool that synchronizes GitHub issues with Jira.

## Features

- Synchronizes GitHub issues with Jira tickets
- Routes issues to specific Jira boards using labels

## Installation

### Prerequisites

- Go 1.16 or higher
- GitHub access token with repo scope
- Jira access token

### Building from source

```bash
git clone https://github.com/yourusername/glue.git
cd glue
go build -o glue
```

### Environment variables

The following environment variables need to be set:

```bash
# GitHub
export GITHUB_TOKEN=your_github_token

# For JIRA integration
export JIRA_URL=https://your-domain.atlassian.net
export JIRA_USERNAME=your_email@example.com
export JIRA_TOKEN=your_jira_api_token
```

## Usage

### Using labels to route issues to specific Jira boards

A GitHub issue MUST have a `jira-project: BOARD_NAME` label to specify which Jira project board the issue should be created on. For example:

- Add `jira-project: LEGGCP` to create the Jira ticket on the LEGGCP board
- Add `jira-project: LEGAWS` to create the Jira ticket on the LEGAWS board

Issues without a `jira-project:` label will be skipped during synchronization.

### Synchronizing GitHub issues with Jira

To synchronize all GitHub issues with Jira:

```bash
glue jira --repository username/repo
```

This will:
1. Find all GitHub issues with a `jira-project:` label
2. Create Jira tickets on the specified boards if they don't already exist
3. Add a `jira-id: PROJECT-123` label to each synchronized GitHub issue

## Issue typing

Issues are categorized based on their GitHub labels:
- GitHub issues with a `type: feature` label will be created as "Feature" type in Jira
- GitHub issues with a `type: story` label will be created as "Story" type in Jira
- If no type label is present, they'll default to "Story" type
