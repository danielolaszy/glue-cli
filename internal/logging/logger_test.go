package logging

import (
	"bytes"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestSetupLogger(t *testing.T) {
	// Save original logger to restore later
	originalLogger := defaultLogger

	// Defer restoration of the original logger
	defer func() {
		defaultLogger = originalLogger
	}()

	testCases := []struct {
		name           string
		level          LogLevel
		expectedLevel  slog.Level
	}{
		{
			name:          "Debug level",
			level:         LevelDebug,
			expectedLevel: slog.LevelDebug,
		},
		{
			name:          "Info level",
			level:         LevelInfo,
			expectedLevel: slog.LevelInfo,
		},
		{
			name:          "Warn level",
			level:         LevelWarn,
			expectedLevel: slog.LevelWarn,
		},
		{
			name:          "Error level",
			level:         LevelError,
			expectedLevel: slog.LevelError,
		},
		{
			name:          "Invalid level defaults to Info",
			level:         LogLevel("invalid"),
			expectedLevel: slog.LevelInfo,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup logger with a buffer to capture output
			var buf bytes.Buffer
			SetupLogger(&buf, tc.level)

			// Verify logger was set
			if defaultLogger == nil {
				t.Fatal("defaultLogger is nil after setup")
			}

			// Test logging
			Info("test message")

			// Verify output contains expected level
			output := buf.String()
			t.Logf("Log output: %s", output)

			// The output format could change, but it should contain the level in some form
			if tc.expectedLevel <= slog.LevelInfo && !strings.Contains(output, "INFO") && !strings.Contains(output, "info") {
				t.Errorf("Expected INFO level in output, got: %s", output)
			}
		})
	}
}

func TestMaskSensitive(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Empty string",
			input:    "",
			expected: "<not set>",
		},
		{
			name:     "Short string",
			input:    "abc",
			expected: "<set>",
		},
		{
			name:     "Exactly 4 characters",
			input:    "abcd",
			expected: "<set>",
		},
		{
			name:     "Long string",
			input:    "abcdefghijklm",
			expected: "abcd...***",
		},
		{
			name:     "Token-like string",
			input:    "2Dn5j8fk39Dkf0s",
			expected: "2Dn5...***",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := MaskSensitive(tc.input)
			if result != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestLoggingFunctions(t *testing.T) {
	// Save original logger and env var
	originalLogger := defaultLogger
	origEnv := os.Getenv("LOG_LEVEL")

	// Restore original state after test
	defer func() {
		defaultLogger = originalLogger
		os.Setenv("LOG_LEVEL", origEnv)
	}()

	// Test all logging functions with a buffer
	var buf bytes.Buffer
	SetupLogger(&buf, LevelDebug) // Set to debug to capture all levels

	tests := []struct {
		name     string
		logFunc  func(string, ...any)
		level    string
		message  string
	}{
		{
			name:     "Debug logging",
			logFunc:  Debug,
			level:    "DEBUG",
			message:  "debug message",
		},
		{
			name:     "Info logging",
			logFunc:  Info,
			level:    "INFO",
			message:  "info message",
		},
		{
			name:     "Warn logging",
			logFunc:  Warn,
			level:    "WARN",
			message:  "warn message",
		},
		{
			name:     "Error logging",
			logFunc:  Error,
			level:    "ERROR",
			message:  "error message",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Clear buffer
			buf.Reset()

			// Call the log function
			tc.logFunc(tc.message, "key", "value")

			// Check output
			output := buf.String()
			if !strings.Contains(strings.ToUpper(output), tc.level) {
				t.Errorf("Expected log level %s in output, got: %s", tc.level, output)
			}
			if !strings.Contains(output, tc.message) {
				t.Errorf("Expected message %q in output, got: %s", tc.message, output)
			}
			if !strings.Contains(output, "key") || !strings.Contains(output, "value") {
				t.Errorf("Expected key-value pair in output, got: %s", output)
			}
		})
	}
}

func TestGetLogger(t *testing.T) {
	// Ensure the logger exists
	logger := GetLogger()
	if logger == nil {
		t.Fatal("GetLogger() returned nil")
	}
}

func TestInitFromEnvironment(t *testing.T) {
	// Save original logger and env var
	originalLogger := defaultLogger
	origEnv := os.Getenv("LOG_LEVEL")

	// Restore original state after test
	defer func() {
		defaultLogger = originalLogger
		os.Setenv("LOG_LEVEL", origEnv)
	}()

	testCases := []struct {
		name      string
		envValue  string
		shouldLog map[slog.Level]bool // Map of levels and whether they should be logged
	}{
		{
			name:     "Debug level from env",
			envValue: "debug",
			shouldLog: map[slog.Level]bool{
				slog.LevelDebug: true,
				slog.LevelInfo:  true,
				slog.LevelWarn:  true,
				slog.LevelError: true,
			},
		},
		{
			name:     "Info level from env",
			envValue: "info",
			shouldLog: map[slog.Level]bool{
				slog.LevelDebug: false,
				slog.LevelInfo:  true,
				slog.LevelWarn:  true,
				slog.LevelError: true,
			},
		},
		{
			name:     "Warn level from env",
			envValue: "warn",
			shouldLog: map[slog.Level]bool{
				slog.LevelDebug: false,
				slog.LevelInfo:  false,
				slog.LevelWarn:  true,
				slog.LevelError: true,
			},
		},
		{
			name:     "Error level from env",
			envValue: "error",
			shouldLog: map[slog.Level]bool{
				slog.LevelDebug: false,
				slog.LevelInfo:  false,
				slog.LevelWarn:  false,
				slog.LevelError: true,
			},
		},
		{
			name:     "Empty env defaults to Info",
			envValue: "",
			shouldLog: map[slog.Level]bool{
				slog.LevelDebug: false,
				slog.LevelInfo:  true,
				slog.LevelWarn:  true,
				slog.LevelError: true,
			},
		},
		{
			name:     "Invalid env defaults to Info",
			envValue: "invalid",
			shouldLog: map[slog.Level]bool{
				slog.LevelDebug: false,
				slog.LevelInfo:  true,
				slog.LevelWarn:  true,
				slog.LevelError: true,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set environment variable
			os.Setenv("LOG_LEVEL", tc.envValue)

			// Create a buffer to capture output
			var buf bytes.Buffer

			// Call init indirectly by setting up logger
			SetupLogger(&buf, LogLevel(tc.envValue))

			// Test each logging level
			levels := map[slog.Level]func(string, ...any){
				slog.LevelDebug: Debug,
				slog.LevelInfo:  Info,
				slog.LevelWarn:  Warn,
				slog.LevelError: Error,
			}

			levelNames := map[slog.Level]string{
				slog.LevelDebug: "DEBUG",
				slog.LevelInfo:  "INFO",
				slog.LevelWarn:  "WARN",
				slog.LevelError: "ERROR",
			}

			for level, logFunc := range levels {
				buf.Reset()
				logFunc("test message for level", "level", levelNames[level])
				output := buf.String()

				shouldLog := tc.shouldLog[level]
				didLog := output != "" && strings.Contains(output, "test message for level")

				if shouldLog != didLog {
					if shouldLog {
						t.Errorf("Expected %s level to be logged with LOG_LEVEL=%s, but it wasn't", 
							levelNames[level], tc.envValue)
					} else {
						t.Errorf("Expected %s level NOT to be logged with LOG_LEVEL=%s, but it was", 
							levelNames[level], tc.envValue)
					}
				}
			}
		})
	}
} 