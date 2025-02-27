# Glue CLI

Glue is a command-line tool that synchronizes GitHub issues with project management tools like JIRA and Trello.

## Features

- Initializes GitHub repositories with necessary labels
- Synchronizes GitHub issues with JIRA tickets or Trello cards
- Provides status information about synchronization
- Supports different issue types (Story, Feature)

## Installation

### Prerequisites

- Go 1.16 or higher
- GitHub access token with repo scope
- JIRA access token or Trello API key and token

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

# For Trello integration
export TRELLO_API_KEY=your_trello_api_key
export TRELLO_TOKEN=your_trello_token
```

## Usage

### Initialize GitHub repository with required labels

```bash
glue github init --repository username/repo
```

This command will create the following labels in your GitHub repository if they don't already exist:
- `story`: For user stories
- `feature`: For feature requests
- `glued`: For issues that have been synchronized with your project management tool

### Synchronize GitHub issues with JIRA

```bash
glue jira sync --repository username/repo --board PROJECT_NAME
```

This command will:
1. Get all open GitHub issues without the `glued` label
2. Create JIRA tickets for each issue
3. Add the `glued` label to the GitHub issues

### Synchronize GitHub issues with Trello

```bash
glue trello sync --repository username/repo --board "Board Name"
```

This command works similarly to the JIRA sync command but creates Trello cards instead.

### Check synchronization status

```bash
# For JIRA
glue jira status --repository username/repo --board PROJECT_NAME

# For Trello
glue trello status --repository username/repo --board "Board Name"
```

These commands will show statistics about synchronized and non-synchronized issues.

## Development

### Project Structure

```
glue/
├── cmd/             # Command implementations
├── internal/        # Internal packages
│   ├── github/      # GitHub API client
│   ├── jira/        # JIRA API client
│   ├── trello/      # Trello API client
│   └── common/      # Shared utilities
├── pkg/             # Public packages
│   └── models/      # Data models
└── main.go          # Entry point
```

### Testing

```bash
go test ./...
```

## License

MIT