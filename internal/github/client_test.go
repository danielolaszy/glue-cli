package github

import (
	"os"
	"testing"
)

func TestNewClient(t *testing.T) {
	// Save the original environment variable to restore it later
	originalToken := os.Getenv("GITHUB_TOKEN")
	defer os.Setenv("GITHUB_TOKEN", originalToken)

	// Test with token set
	os.Setenv("GITHUB_TOKEN", "test-token")
	client := NewClient()
	if client == nil {
		t.Error("NewClient() should not return nil when GITHUB_TOKEN is set")
	}

	// Test with token not set
	os.Setenv("GITHUB_TOKEN", "")
	client = NewClient()
	if client == nil {
		t.Error("NewClient() should not return nil even when GITHUB_TOKEN is not set")
	}
}

func TestGetLabelColor(t *testing.T) {
	tests := []struct {
		label    string
		expected string
	}{
		{"story", "0075ca"},
		{"feature", "7057ff"},
		{"glued", "d4c5f9"},
		{"unknown", "cccccc"},
	}

	for _, test := range tests {
		color := getLabelColor(test.label)
		if color != test.expected {
			t.Errorf("getLabelColor(%s) = %s; expected %s", test.label, color, test.expected)
		}
	}
}

func TestGetLabelDescription(t *testing.T) {
	tests := []struct {
		label    string
		expected string
	}{
		{"story", "User story"},
		{"feature", "Feature request"},
		{"glued", "Synchronized with project management tool"},
		{"unknown", ""},
	}

	for _, test := range tests {
		description := getLabelDescription(test.label)
		if description != test.expected {
			t.Errorf("getLabelDescription(%s) = %s; expected %s", test.label, description, test.expected)
		}
	}
}
