package logging

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupLogger(t *testing.T) {
	tests := []struct {
		name     string
		level    LogLevel
		message  string
		wantLogs bool
	}{
		{
			name:     "Debug level",
			level:    LevelDebug,
			message:  "test message",
			wantLogs: true,
		},
		{
			name:     "Info level",
			level:    LevelInfo,
			message:  "test message",
			wantLogs: true,
		},
		{
			name:     "Warn level",
			level:    LevelWarn,
			message:  "test message",
			wantLogs: false,
		},
		{
			name:     "Error level",
			level:    LevelError,
			message:  "test message",
			wantLogs: false,
		},
		{
			name:     "Invalid level defaults to Info",
			level:    LogLevel("invalid"),
			message:  "test message",
			wantLogs: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			SetupLogger(&buf, tt.level)

			Info(tt.message)

			output := buf.String()
			t.Logf("Log output: %s", output)

			if tt.wantLogs {
				assert.Contains(t, output, tt.message)
				assert.Contains(t, output, "level=INFO")
			} else {
				assert.Empty(t, output)
			}
		})
	}
}

func TestMaskSensitive(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "Empty string",
			input: "",
			want:  "<not set>",
		},
		{
			name:  "Short string",
			input: "abc",
			want:  "<set>",
		},
		{
			name:  "Exactly 4 characters",
			input: "abcd",
			want:  "<set>",
		},
		{
			name:  "Long string",
			input: "abcdefghijk",
			want:  "abcd...***",
		},
		{
			name:  "Token-like string",
			input: "ghp_1234567890abcdef",
			want:  "ghp_...***",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MaskSensitive(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLoggingFunctions(t *testing.T) {
	tests := []struct {
		name     string
		logFunc  func(string, ...interface{})
		level    LogLevel
		wantLogs bool
	}{
		{
			name:     "Debug logging",
			logFunc:  Debug,
			level:    LevelDebug,
			wantLogs: true,
		},
		{
			name:     "Info logging",
			logFunc:  Info,
			level:    LevelInfo,
			wantLogs: true,
		},
		{
			name:     "Warn logging",
			logFunc:  Warn,
			level:    LevelWarn,
			wantLogs: true,
		},
		{
			name:     "Error logging",
			logFunc:  Error,
			level:    LevelError,
			wantLogs: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			SetupLogger(&buf, tt.level)

			tt.logFunc("test message")

			output := buf.String()
			if tt.wantLogs {
				assert.Contains(t, output, "test message")
				assert.Contains(t, output, strings.ToUpper(string(tt.level)))
			} else {
				assert.Empty(t, output)
			}
		})
	}
}

func TestGetLogger(t *testing.T) {
	logger := GetLogger()
	assert.NotNil(t, logger)
}

func TestSetupLoggerFromEnv(t *testing.T) {
	tests := []struct {
		name      string
		envLevel  string
		wantLevel LogLevel
	}{
		{
			name:      "Debug level from env",
			envLevel:  "debug",
			wantLevel: LevelDebug,
		},
		{
			name:      "Info level from env",
			envLevel:  "info",
			wantLevel: LevelInfo,
		},
		{
			name:      "Warn level from env",
			envLevel:  "warn",
			wantLevel: LevelWarn,
		},
		{
			name:      "Error level from env",
			envLevel:  "error",
			wantLevel: LevelError,
		},
		{
			name:      "Empty env defaults to Info",
			envLevel:  "",
			wantLevel: LevelInfo,
		},
		{
			name:      "Invalid env defaults to Info",
			envLevel:  "invalid",
			wantLevel: LevelInfo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origLevel := os.Getenv("LOG_LEVEL")
			require.NoError(t, os.Setenv("LOG_LEVEL", tt.envLevel))

			var buf bytes.Buffer
			SetupLogger(&buf, tt.wantLevel)

			Info("test message")
			output := buf.String()

			if tt.wantLevel == LevelInfo || tt.wantLevel == LevelDebug {
				assert.Contains(t, output, "test message")
			} else {
				assert.Empty(t, output)
			}

			require.NoError(t, os.Setenv("LOG_LEVEL", origLevel))
		})
	}
} 