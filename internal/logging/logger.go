// Package logging provides centralized logging functionality for the application.
package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// LogLevel represents the logging level.
type LogLevel string

const (
	// LevelDebug for detailed troubleshooting information.
	LevelDebug LogLevel = "debug"
	// LevelInfo for general operational information.
	LevelInfo LogLevel = "info"
	// LevelWarn for potentially harmful situations.
	LevelWarn LogLevel = "warn"
	// LevelError for error events that might still allow the application to continue.
	LevelError LogLevel = "error"
)

var (
	// defaultLogger is the default logger instance.
	defaultLogger *slog.Logger
)

// init initializes the default logger.
func init() {
	// Get log level from environment variable, default to "info"
	logLevelStr := strings.ToLower(os.Getenv("LOG_LEVEL"))
	if logLevelStr == "" {
		logLevelStr = string(LevelInfo)
	}

	// Set up the logger
	SetupLogger(os.Stdout, LogLevel(logLevelStr))
}

// SetupLogger configures the logger with the specified output and level.
func SetupLogger(w io.Writer, level LogLevel) {
	var logLevel slog.Level
	switch level {
	case LevelDebug:
		logLevel = slog.LevelDebug
	case LevelInfo:
		logLevel = slog.LevelInfo
	case LevelWarn:
		logLevel = slog.LevelWarn
	case LevelError:
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	handler := slog.NewTextHandler(w, opts)
	defaultLogger = slog.New(handler)
	slog.SetDefault(defaultLogger)
}

// Debug logs a message at debug level.
func Debug(msg string, args ...any) {
	defaultLogger.Debug(msg, args...)
}

// Info logs a message at info level.
func Info(msg string, args ...any) {
	defaultLogger.Info(msg, args...)
}

// Warn logs a message at warn level.
func Warn(msg string, args ...any) {
	defaultLogger.Warn(msg, args...)
}

// Error logs a message at error level.
func Error(msg string, args ...any) {
	defaultLogger.Error(msg, args...)
}

// GetLogger returns the default logger.
func GetLogger() *slog.Logger {
	return defaultLogger
}

// MaskSensitive masks sensitive data for logging.
func MaskSensitive(value string) string {
	if value == "" {
		return "<not set>"
	}
	if len(value) <= 4 {
		return "<set>"
	}
	return value[:4] + "..." + strings.Repeat("*", 3)
} 