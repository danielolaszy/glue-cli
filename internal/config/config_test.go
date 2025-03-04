package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadGitHubConfig(t *testing.T) {
	tests := []struct {
		name     string
		domain   string
		token    string
		wantErr  bool
	}{
		{
			name:     "Explicit github.com",
			domain:   "github.com",
			token:    "test-token",
			wantErr:  false,
		},
		{
			name:     "Custom GitHub domain",
			domain:   "github.example.com",
			token:    "test-token",
			wantErr:  false,
		},
		{
			name:     "Empty domain should default to github.com",
			domain:   "",
			token:    "test-token",
			wantErr:  false,
		},
		{
			name:     "Missing token",
			domain:   "github.com",
			token:    "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original env vars
			origDomain := os.Getenv("GITHUB_DOMAIN")
			origToken := os.Getenv("GITHUB_TOKEN")

			// Set test env vars
			require.NoError(t, os.Setenv("GITHUB_DOMAIN", tt.domain))
			require.NoError(t, os.Setenv("GITHUB_TOKEN", tt.token))

			// Run test
			config, err := LoadConfig()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, config)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, config)
				if tt.domain == "" {
					assert.Equal(t, "github.com", config.GitHub.Domain)
				} else {
					assert.Equal(t, tt.domain, config.GitHub.Domain)
				}
				assert.Equal(t, tt.token, config.GitHub.Token)
			}

			// Restore original env vars
			require.NoError(t, os.Setenv("GITHUB_DOMAIN", origDomain))
			require.NoError(t, os.Setenv("GITHUB_TOKEN", origToken))
		})
	}
}

func TestValidateJiraConfig(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		username string
		token    string
		wantErr  bool
	}{
		{
			name:     "All fields present",
			baseURL:  "https://jira.example.com",
			username: "test-user",
			token:    "test-token",
			wantErr:  false,
		},
		{
			name:     "Missing base URL",
			baseURL:  "",
			username: "test-user",
			token:    "test-token",
			wantErr:  true,
		},
		{
			name:     "Missing username",
			baseURL:  "https://jira.example.com",
			username: "",
			token:    "test-token",
			wantErr:  true,
		},
		{
			name:     "Missing token",
			baseURL:  "https://jira.example.com",
			username: "test-user",
			token:    "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				Jira: JiraConfig{
					BaseURL:  tt.baseURL,
					Username: tt.username,
					Token:    tt.token,
				},
			}

			err := ValidateJiraConfig(config)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
} 