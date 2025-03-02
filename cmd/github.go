package cmd

import (
	"fmt"
	"github.com/danielolaszy/glue/internal/github"
	"github.com/danielolaszy/glue/internal/logging"
	"regexp"

	"github.com/spf13/cobra"
)

var githubCmd = &cobra.Command{
	Use:   "github",
	Short: "Initialize GitHub repository",
	Long: `Initialize a GitHub repository with the necessary labels for integration.
	
This command creates the required labels in your GitHub repository:
- 'type: feature' - For feature requests
- 'type: story' - For user stories
- 'jira-project: PROJECT_KEY' - To specify which JIRA board to create tickets on (required)`,
}

func init() {
	// Add init subcommand
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize GitHub repository with required labels",
		Long: `Initialize a GitHub repository with the necessary labels for Glue integration.
		
This creates the following labels in your GitHub repository:
- 'type: feature' - For issues that should be created as Feature type in JIRA
- 'type: story' - For issues that should be created as Story type in JIRA
- 'jira-project: PROJECT_KEY' - (REQUIRED) Specifies which JIRA board to create tickets on

A 'jira-project:' label is REQUIRED on each GitHub issue you want to synchronize.
Without this label, issues will be skipped during synchronization.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			repository, err := cmd.Flags().GetString("repository")
			if err != nil {
				return err
			}
			
			if repository == "" {
				return fmt.Errorf("repository flag is required")
			}
			
			board, err := cmd.Flags().GetString("board")
			if err != nil {
				return err
			}
			
			if board == "" {
				return fmt.Errorf("board flag is required - you must specify a default JIRA board with --board")
			}

			// Initialize GitHub client
			githubClient, err := github.NewClient()
			if err != nil {
				return fmt.Errorf("failed to initialize GitHub client: %w", err)
			}
			
			// Create the required labels
			err = createRequiredLabels(githubClient, repository, board)
			if err != nil {
				return err
			}
			
			logging.Info("GitHub repository successfully initialized", "repository", repository)
			fmt.Printf("\nSuccessfully created labels including 'jira-project: %s'.\n", board)
			fmt.Println("You can now add GitHub issues with this label to have them synchronized with JIRA.")
			return nil
		},
	}
	
	// Add board flag for creating jira-project label for the default board
	initCmd.Flags().StringP("board", "b", "", "JIRA board/project key to create a jira-project label for (REQUIRED)")
	
	// Add the test subcommand
	testCmd := &cobra.Command{
		Use:   "test",
		Short: "Test GitHub integration",
		Long: `Test GitHub integration by verifying label access and detection.
		
This command verifies that the GitHub client can access the repository and detect labels.
It will check if the specified issue has the required jira-project label.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			repository, err := cmd.Flags().GetString("repository")
			if err != nil {
				return err
			}
			
			if repository == "" {
				return fmt.Errorf("repository flag is required")
			}
			
			issue, err := cmd.Flags().GetInt("issue")
			if err != nil {
				return err
			}
			
			if issue <= 0 {
				return fmt.Errorf("issue number must be greater than 0")
			}

			githubClient, err := github.NewClient()
			if err != nil {
				return fmt.Errorf("failed to initialize GitHub client: %w", err)
			}

			// Test jira-project label first since it's required
			jiraProjectPattern := regexp.MustCompile(`^jira-project: ([A-Z0-9]+)$`)
			hasJiraProject, err := githubClient.HasLabelMatching(repository, issue, jiraProjectPattern)
			if err != nil {
				return err
			}
			
			if hasJiraProject {
				// Get all labels and find the jira-project one
				labels, err := githubClient.GetLabelsForIssue(repository, issue)
				if err != nil {
					return err
				}
				
				for _, label := range labels {
					if jiraProjectPattern.MatchString(label) {
						logging.Info("GitHub issue has jira-project label ✓", "issue", issue, "label", label)
						fmt.Printf("Issue #%d has jira-project label: %s ✓\n", issue, label)
						break
					}
				}
			} else {
				logging.Warn("GitHub issue does not have a jira-project label ✗", "issue", issue)
				fmt.Printf("Issue #%d DOES NOT have a jira-project label ✗\n", issue)
				fmt.Println("This issue will be SKIPPED during synchronization!")
				fmt.Println("Please add a 'jira-project: BOARD_NAME' label to this issue.")
			}

			// Create a pattern to match JIRA ID labels and extract the project key
			jiraPattern := regexp.MustCompile(`^jira-id: ([A-Z]+)-\d+$`)
			matching, err := githubClient.HasLabelMatching(repository, issue, jiraPattern)
			if err != nil {
				return err
			}
			
			if matching {
				logging.Info("GitHub issue has a JIRA ID label", "issue", issue)
				fmt.Printf("Issue #%d has already been synchronized with JIRA ✓\n", issue)
			} else {
				logging.Info("GitHub issue does not have a JIRA ID label", "issue", issue)
				fmt.Printf("Issue #%d has not yet been synchronized with JIRA\n", issue)
			}

			// Test feature label
			hasFeature, err := githubClient.HasLabel(repository, issue, "type: feature")
			if err != nil {
				return err
			}
			
			// Test story label
			hasStory, err := githubClient.HasLabel(repository, issue, "type: story")
			if err != nil {
				return err
			}
			
			if hasFeature {
				logging.Info("GitHub issue has 'type: feature' label", "issue", issue)
				fmt.Printf("Issue #%d will be created as a Feature in JIRA\n", issue)
			} else if hasStory {
				logging.Info("GitHub issue has 'type: story' label", "issue", issue)
				fmt.Printf("Issue #%d will be created as a Story in JIRA\n", issue)
			} else {
				logging.Info("GitHub issue has no type label", "issue", issue)
				fmt.Printf("Issue #%d will be created with the default type in JIRA\n", issue)
			}

			return nil
		},
	}
	
	// Add issue flag for the test command
	testCmd.Flags().IntP("issue", "i", 0, "GitHub issue number to test")
	
	// Add subcommands to the github command
	githubCmd.AddCommand(initCmd)
	githubCmd.AddCommand(testCmd)
}

// createRequiredLabels creates the standard labels required for the Glue integration
func createRequiredLabels(client *github.Client, repository, board string) error {
	// Define the required labels
	labels := []string{
		"type: feature",
		"type: story",
	}
	
	// Always add jira-project label since it's now required
	jiraProjectLabel := fmt.Sprintf("jira-project: %s", board)
	labels = append(labels, jiraProjectLabel)
	
	// Create each label on a test issue
	// Note: Since we can't create labels directly in GitHub API,
	// we need an issue to add labels to. This is a limitation of the GitHub API.
	// In a real-world scenario, we might want to create our first issue automatically
	// or check if we have any existing issues to add the labels to.
	logging.Info("verifying repository access", "repository", repository)
	
	// Get all issues to find the first open one
	issues, err := client.GetAllIssues(repository)
	if err != nil {
		return fmt.Errorf("failed to access repository: %w", err)
	}
	
	if len(issues) == 0 {
		return fmt.Errorf("repository has no open issues, please create at least one issue first")
	}
	
	// Use the first issue to add the labels
	firstIssue := issues[0]
	
	logging.Info("creating required labels", "repository", repository, "issue", firstIssue.Number)
	
	// Add the labels to the issue
	err = client.AddLabels(repository, firstIssue.Number, labels...)
	if err != nil {
		return fmt.Errorf("failed to create labels: %w", err)
	}
	
	logging.Info("successfully created labels", "repository", repository, "labels", labels)
	return nil
}
