package common

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

// Logger provides logging functionality
type Logger struct {
	infoLog  *log.Logger
	errorLog *log.Logger
	debugLog *log.Logger
}

// NewLogger creates a new Logger
func NewLogger(appName string) (*Logger, error) {
	// Create logs directory if it doesn't exist
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %v", err)
	}

	logsDir := filepath.Join(homeDir, "."+appName, "logs")
	err = os.MkdirAll(logsDir, 0755)
	if err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %v", err)
	}

	// Create log file with current date
	logFileName := fmt.Sprintf("%s-%s.log", appName, time.Now().Format("2006-01-02"))
	logFilePath := filepath.Join(logsDir, logFileName)

	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %v", err)
	}

	// Create multiwriter to log to both file and stderr
	infoWriter := io.MultiWriter(os.Stdout, logFile)
	errorWriter := io.MultiWriter(os.Stderr, logFile)
	debugWriter := logFile

	// Create loggers
	infoLog := log.New(infoWriter, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)
	errorLog := log.New(errorWriter, "ERROR: ", log.Ldate|log.Ltime|log.Lshortfile)
	debugLog := log.New(debugWriter, "DEBUG: ", log.Ldate|log.Ltime|log.Lshortfile)

	return &Logger{
		infoLog:  infoLog,
		errorLog: errorLog,
		debugLog: debugLog,
	}, nil
}

// Info logs an informational message
func (l *Logger) Info(format string, v ...interface{}) {
	l.infoLog.Printf(format, v...)
}

// Error logs an error message
func (l *Logger) Error(format string, v ...interface{}) {
	l.errorLog.Printf(format, v...)
}

// Debug logs a debug message
func (l *Logger) Debug(format string, v ...interface{}) {
	l.debugLog.Printf(format, v...)
}
