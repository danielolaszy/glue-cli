// Package config provides centralized configuration management for the application.
package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config holds all configuration parameters for the application.
type Config struct {
	GitHub GitHubConfig
	Jira   JiraConfig
}

// GitHubConfig holds GitHub specific configuration.
type GitHubConfig struct {
	Token string
}

// JiraConfig holds JIRA specific configuration.
type JiraConfig struct {
	URL      string
	Username string
	Token    string
}

// LoadConfig initializes and loads configuration from environment variables.
func LoadConfig() (*Config, error) {
	// Initialize Viper for environment variables
	v := viper.New()
	v.SetEnvPrefix("")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Map specific environment variables
	v.BindEnv("github.token", "GITHUB_TOKEN")
	v.BindEnv("jira.url", "JIRA_URL")
	v.BindEnv("jira.username", "JIRA_USERNAME")
	v.BindEnv("jira.token", "JIRA_TOKEN")

	// Create config structure
	config := &Config{
		GitHub: GitHubConfig{
			Token: v.GetString("github.token"),
		},
		Jira: JiraConfig{
			URL:      v.GetString("jira.url"),
			Username: v.GetString("jira.username"),
			Token:    v.GetString("jira.token"),
		},
	}

	// Validate configuration
	if err := validateConfig(config); err != nil {
		return nil, err
	}

	return config, nil
}

// validateConfig ensures that all required configuration values are provided.
func validateConfig(config *Config) error {
	var missingVars []string

	// GitHub validation
	if config.GitHub.Token == "" {
		missingVars = append(missingVars, "GITHUB_TOKEN")
	}

	return nil
}

// ValidateJiraConfig validates JIRA-specific configuration.
func ValidateJiraConfig(config *Config) error {
	var missingVars []string

	// JIRA validation
	if config.Jira.URL == "" {
		missingVars = append(missingVars, "JIRA_URL")
	}
	if config.Jira.Username == "" {
		missingVars = append(missingVars, "JIRA_USERNAME")
	}
	if config.Jira.Token == "" {
		missingVars = append(missingVars, "JIRA_TOKEN")
	}

	if len(missingVars) > 0 {
		return fmt.Errorf("missing required environment variables: %v", missingVars)
	}

	return nil
} 