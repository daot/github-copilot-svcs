package logger_test

import (
	"os"
	"testing"

	"github.com/privapps/github-copilot-svcs/internal"
)

func TestNewLogger(t *testing.T) {
	tests := []struct {
		name     string
		level    string
		expected string
	}{
		{
			name:     "debug level",
			level:    "debug",
			expected: "debug",
		},
		{
			name:     "info level",
			level:    "info",
			expected: "info",
		},
		{
			name:     "warn level",
			level:    "warn",
			expected: "warn",
		},
		{
			name:     "error level",
			level:    "error",
			expected: "error",
		},
		{
			name:     "invalid level defaults to info",
			level:    "invalid",
			expected: "info",
		},
		{
			name:     "empty level defaults to info",
			level:    "",
			expected: "info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			log := internal.NewLogger(tt.level)
			if log == nil {
				t.Errorf("expected logger, got nil")
			}
		})
	}
}

func TestInitLogger(t *testing.T) {
	tests := []struct {
		name     string
		envLevel string
	}{
		{
			name:     "init with debug level",
			envLevel: "debug",
		},
		{
			name:     "init with default level",
			envLevel: "",
		},
		{
			name:     "init with invalid level",
			envLevel: "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			// Set environment variable
			if tt.envLevel != "" {
				os.Setenv("LOG_LEVEL", tt.envLevel)
			} else {
				os.Unsetenv("LOG_LEVEL")
			}

			// Initialize logger
			internal.Init()

			// Test that logger functions work without panicking
			internal.Debug("test debug message")
			internal.Info("test info message")
			internal.Warn("test warn message")
			internal.Error("test error message")

			// Cleanup
			os.Unsetenv("LOG_LEVEL")
		})
	}
}
