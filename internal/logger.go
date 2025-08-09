package internal

import (
	"log/slog"
	"os"
	"strings"
)

const (
	defaultLogLevel = "info"
)

// Logger wraps slog.Logger for structured logging
type Logger struct {
	*slog.Logger
}

// NewLogger creates a new logger with the specified level
func NewLogger(level string) *Logger {
	var logLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		logLevel = slog.LevelDebug
	case defaultLogLevel:
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	handler := slog.NewTextHandler(os.Stdout, opts)
	return &Logger{slog.New(handler)}
}

var logger *Logger

// Init initializes the global logger from environment variable
func Init() {
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = defaultLogLevel
	}
	logger = NewLogger(logLevel)
}

// Debug logs a debug message
func Debug(msg string, args ...any) {
	if logger != nil {
		logger.Debug(msg, args...)
	}
}

// Info logs an info message
func Info(msg string, args ...any) {
	if logger != nil {
		logger.Info(msg, args...)
	}
}

// Warn logs a warning message
func Warn(msg string, args ...any) {
	if logger != nil {
		logger.Warn(msg, args...)
	}
}

// Error logs an error message
func Error(msg string, args ...any) {
	if logger != nil {
		logger.Error(msg, args...)
	}
}

// GetLogger returns the global logger instance
func GetLogger() *Logger {
	return logger
}
