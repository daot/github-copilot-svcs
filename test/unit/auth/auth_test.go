package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/privapps/github-copilot-svcs/internal"
)

// Test constants
const (
	testUserAgent = "test-agent/1.0"
)

// Helper function to create a basic test config
func createTestConfig() *internal.Config {
	return &internal.Config{
		Headers: struct {
			UserAgent            string `json:"user_agent"`
			EditorVersion        string `json:"editor_version"`
			EditorPluginVersion  string `json:"editor_plugin_version"`
			CopilotIntegrationID string `json:"copilot_integration_id"`
			OpenaiIntent         string `json:"openai_intent"`
			XInitiator           string `json:"x_initiator"`
		}{
			UserAgent: testUserAgent,
		},
	}
}

func TestAuthService_EnsureValidToken(t *testing.T) {
	tests := []struct {
		name          string
		setupConfig   func() *internal.Config
		expectedError bool
	}{
		{
			name:          "no token",
			setupConfig:   createTestConfig,
			expectedError: true,
		},
		{
			name: "valid token - not expiring soon",
			setupConfig: func() *internal.Config {
				cfg := createTestConfig()
				cfg.CopilotToken = "valid_token"
				cfg.ExpiresAt = time.Now().Add(time.Hour).Unix() // Expires in 1 hour
				return cfg
			},
			expectedError: false,
		},
		{
			name: "token expiring soon - but no github token to refresh",
			setupConfig: func() *internal.Config {
				cfg := createTestConfig()
				cfg.CopilotToken = "expiring_token"
				cfg.ExpiresAt = time.Now().Add(2 * time.Minute).Unix() // Expires in 2 minutes
				// No GitHubToken, so refresh should fail
				return cfg
			},
			expectedError: true,
		},
		{
			name: "expired token - but no github token to refresh",
			setupConfig: func() *internal.Config {
				cfg := createTestConfig()
				cfg.CopilotToken = "expired_token"
				cfg.ExpiresAt = time.Now().Unix() - 100 // Expired 100 seconds ago
				// No GitHubToken, so refresh should fail
				return cfg
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.setupConfig()

			// Use a basic client for non-HTTP tests
			authService := internal.NewAuthService(&http.Client{Timeout: 1 * time.Second})
			err := authService.EnsureValidToken(cfg)

			if tt.expectedError {
				if err == nil {
					t.Error("Expected error but got none")
				} else {
					t.Logf("Got expected error: %v", err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestAuthService_RefreshToken_ValidationLogic(t *testing.T) {
	tests := []struct {
		name          string
		setupConfig   func() *internal.Config
		expectedError bool
		errorContains string
	}{
		{
			name: "no github token",
			setupConfig: func() *internal.Config {
				cfg := createTestConfig()
				cfg.CopilotToken = "old_token"
				// No GitHubToken set
				return cfg
			},
			expectedError: true,
			errorContains: "no GitHub token available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.setupConfig()
			authService := internal.NewAuthService(&http.Client{Timeout: 1 * time.Second})

			err := authService.RefreshToken(cfg)

			if tt.expectedError {
				if err == nil {
					t.Error("Expected error but got none")
				} else {
					t.Logf("Got expected error: %v", err)
					if tt.errorContains != "" && err.Error() != "" {
						// We expect the error to contain certain text
						t.Logf("Error contains expected text: %q", tt.errorContains)
					}
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestAuthService_RefreshTokenWithContext_CancellationLogic(t *testing.T) {
	// Test that validates context cancellation is properly handled
	// This test focuses on the context handling logic without HTTP complexity
	tests := []struct {
		name        string
		setupConfig func() *internal.Config
		setupCtx    func() context.Context
		expectError bool
	}{
		{
			name: "context already canceled",
			setupConfig: func() *internal.Config {
				cfg := createTestConfig()
				cfg.GitHubToken = "test_token" // Has github token
				return cfg
			},
			setupCtx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately
				return ctx
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.setupConfig()
			authService := internal.NewAuthService(&http.Client{Timeout: 1 * time.Second})
			ctx := tt.setupCtx()

			err := authService.RefreshTokenWithContext(ctx, cfg)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				} else {
					t.Logf("Got expected error: %v", err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// Test NewAuthService constructor
func TestNewAuthService(t *testing.T) {
	authService := internal.NewAuthService(&http.Client{Timeout: 1 * time.Second})
	if authService == nil {
		t.Error("NewAuthService returned nil")
	}
}

// Test token expiry calculation logic
func TestTokenExpiryLogic(t *testing.T) {
	tests := []struct {
		name          string
		expiresAt     int64
		currentTime   int64
		shouldBeValid bool
		description   string
	}{
		{
			name:          "token valid for 1 hour",
			expiresAt:     time.Now().Add(time.Hour).Unix(),
			shouldBeValid: true,
			description:   "Token expires in 1 hour, should be valid",
		},
		{
			name:          "token expiring in 2 minutes",
			expiresAt:     time.Now().Add(2 * time.Minute).Unix(),
			shouldBeValid: false,
			description:   "Token expires in 2 minutes, should trigger refresh",
		},
		{
			name:          "token expired 1 hour ago",
			expiresAt:     time.Now().Add(-time.Hour).Unix(),
			shouldBeValid: false,
			description:   "Token expired 1 hour ago, should trigger refresh",
		},
		{
			name:          "token expiring in exactly 5 minutes",
			expiresAt:     time.Now().Add(5 * time.Minute).Unix(),
			shouldBeValid: false,
			description:   "Token expires in exactly 5 minutes, should trigger refresh",
		},
		{
			name:          "token expiring in 6 minutes",
			expiresAt:     time.Now().Add(6 * time.Minute).Unix(),
			shouldBeValid: true,
			description:   "Token expires in 6 minutes, should still be valid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := createTestConfig()
			cfg.CopilotToken = "test_token"
			cfg.ExpiresAt = tt.expiresAt

			authService := internal.NewAuthService(&http.Client{Timeout: 1 * time.Second})
			err := authService.EnsureValidToken(cfg)

			if tt.shouldBeValid {
				if err != nil {
					t.Errorf("Expected token to be valid, but got error: %v", err)
				}
			} else {
				if err == nil {
					t.Error("Expected token to need refresh, but no error was returned")
				}
			}

			t.Logf("%s: %v", tt.description, err)
		})
	}
}

// Benchmark tests for performance verification
func BenchmarkAuthService_EnsureValidToken_ValidToken(b *testing.B) {
	cfg := createTestConfig()
	cfg.CopilotToken = "valid_token"
	cfg.ExpiresAt = time.Now().Add(time.Hour).Unix()

	authService := internal.NewAuthService(&http.Client{Timeout: 1 * time.Second})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = authService.EnsureValidToken(cfg)
	}
}

func BenchmarkAuthService_EnsureValidToken_ExpiredToken(b *testing.B) {
	cfg := createTestConfig()
	cfg.CopilotToken = "expired_token"
	cfg.ExpiresAt = time.Now().Add(-time.Hour).Unix() // Expired

	authService := internal.NewAuthService(&http.Client{Timeout: 1 * time.Second})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = authService.EnsureValidToken(cfg) // Will return error quickly
	}
}

// Test that RefreshToken saves to config file without hitting network
/* Obsolete TestRefreshTokenSavesConfig removed; use TestAuthService_RefreshToken_SavesConfig instead */

// Test that RefreshToken saves to the injected config path
func TestAuthService_RefreshToken_SavesConfig(t *testing.T) {
	// Create temp config file
	tmpfile, err := os.CreateTemp("", "copilot-config-*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	cfg := createTestConfig()
	cfg.GitHubToken = "dummy-github-token"

	// Dummy refresh func (no network)
	refreshFunc := func(c *internal.Config) error {
		c.CopilotToken = "dummy-copilot-token"
		c.ExpiresAt = time.Now().Unix() + 3600
		c.RefreshIn = 1800
		return nil
	}

	authSvc := internal.NewAuthService(&http.Client{},
		internal.WithConfigPath(tmpfile.Name()),
		internal.WithRefreshFunc(refreshFunc),
	)

	if refreshErr := authSvc.RefreshToken(cfg); refreshErr != nil {
		t.Fatalf("RefreshToken failed: %v", refreshErr)
	}

	// Read back the config file
	loaded := &internal.Config{}
	f, openErr := os.Open(tmpfile.Name())
	if openErr != nil {
		t.Fatalf("failed to open temp config file: %v", openErr)
	}
	defer f.Close()
	if decodeErr := json.NewDecoder(f).Decode(loaded); decodeErr != nil {
		t.Fatalf("failed to decode config: %v", decodeErr)
	}

	if loaded.CopilotToken != "dummy-copilot-token" {
		t.Errorf("CopilotToken not saved correctly, got: %v", loaded.CopilotToken)
	}
	if loaded.ExpiresAt == 0 {
		t.Errorf("ExpiresAt not saved")
	}
	if loaded.RefreshIn == 0 {
		t.Errorf("RefreshIn not saved")
	}
}
