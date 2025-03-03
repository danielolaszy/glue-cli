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
export GITHUB_DOMAIN=github.example.com # Optional: Defaults to github.com if not specified

# For JIRA integration
export JIRA_URL=https://your-domain.atlassian.net
export JIRA_USERNAME=your_email@example.com
export JIRA_TOKEN=your_jira_api_token
```

## Usage

### Prerequisites

The Jira project board MUST have a 'Feature' 'Issue type' configured. Otherwise `glue` will not be able to create features.

The GitHub repository must have labels configured.


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

## Issue types

Issues are categorized based on their GitHub labels:
- GitHub issues with a `type: feature` label will be created as "Feature" type in Jira
- GitHub issues with a `type: story` label will be created as "Story" type in Jira
- If no type label is present, they'll default to "Story" type

## Hierarchy Management

`glue` supports creating and maintaining parent-child relationships between issues:

### Creating and Updating Issue Hierarchies

Feature GitHub issues can establish parent-child relationships with other GitHub issues by listing them in a `## Issues` section in the GitHub issue description:

```markdown
## Issues

- https://github.example.com/owner/repo/issues/123
- https://github.example.com/owner/repo/issues/124
```

When syncing, `glue` will:

1. Find all issues listed in the `## Issues` section
2. Create corresponding links between parent and child issues in Jira
3. Report the number of links created in the synchronization summary

### Automatic Link Removal

`glue` automatically detects when issues are removed from the `## Issues` section in GitHub:

1. When a GitHub issue is removed from the `## Issues` section of a GitHub feature
2. The next time synchronization runs, `glue` will detect this change
3. The corresponding link in Jira will be automatically removed
4. The synchronization will report both links created and links removed

This ensures that the issue hierarchy in Jira always matches what's defined in the GitHub issue descriptions.

## Automatic Status Synchronization

`glue` automatically detects when GitHub issues are closed and updates the corresponding Jira tickets:

1. When a GitHub issue with a `jira-id:` label is closed
2. The next time synchronization runs, `glue` will detect the closed issue
3. The corresponding Jira ticket will be automatically transitioned to "Done" or "Closed" status
4. The synchronization will report the number of tickets closed

This ensures that the status of issues in JIRA stays in sync with their status in GitHub.

> [!WARNING]
> `glue` does not re-open a JIRA ticket if a GitHub issue is re-opened.
