package main

import (
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"
)

func printUsage() {
	fmt.Printf("GitHub Copilot SVCS Proxy\n\n")
	fmt.Printf("Usage: %s [command] [options]\n\n", os.Args[0])
	fmt.Printf("Commands:\n")
	fmt.Printf("  start    Start the proxy server (default)\n")
	fmt.Printf("  auth     Authenticate with GitHub Copilot\n")
	fmt.Printf("  status   Show authentication status\n")
	fmt.Printf("  config   Show current configuration\n")
	fmt.Printf("  help     Show this help message\n\n")
	fmt.Printf("Options:\n")
	flag.PrintDefaults()
}

func handleAuth() error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}

	// Initialize timeout configurations before any HTTP operations
	initializeTimeouts(cfg)

	fmt.Println("Starting GitHub Copilot authentication...")
	if err := authenticate(cfg); err != nil {
		return fmt.Errorf("authentication failed: %v", err)
	}

	fmt.Println("Authentication successful!")
	return nil
}

func handleStatus() error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}

	fmt.Printf("Configuration file: %s\n", func() string {
		path, _ := getConfigPath()
		return path
	}())
	fmt.Printf("Port: %d\n", cfg.Port)

	now := getCurrentTime()
	if cfg.CopilotToken != "" {
		fmt.Printf("Authentication: ✓ Authenticated\n")

		timeUntilExpiry := cfg.ExpiresAt - now
		if timeUntilExpiry > 0 {
			minutes := timeUntilExpiry / 60
			seconds := timeUntilExpiry % 60
			fmt.Printf("Token expires: in %dm %ds (%d seconds)\n", minutes, seconds, timeUntilExpiry)

			// Show refresh timing
			if cfg.RefreshIn > 0 {
				refreshThreshold := cfg.RefreshIn / 5 // 20%
				if refreshThreshold < 300 {
					refreshThreshold = 300 // minimum 5 minutes
				}
				if timeUntilExpiry <= refreshThreshold {
					fmt.Printf("Status: ⚠️  Token will be refreshed soon (threshold: %d seconds)\n", refreshThreshold)
				} else {
					fmt.Printf("Status: ✅ Token is healthy\n")
				}
			}
		} else {
			fmt.Printf("Token expires: ⚠️  EXPIRED (%d seconds ago)\n", -timeUntilExpiry)
			fmt.Printf("Status: ❌ Token needs refresh\n")
		}

		fmt.Printf("Has GitHub token: %t\n", cfg.GitHubToken != "")
		if cfg.RefreshIn > 0 {
			fmt.Printf("Refresh interval: %d seconds\n", cfg.RefreshIn)
		}
	} else {
		fmt.Printf("Authentication: ✗ Not authenticated\n")
		fmt.Printf("Run '%s auth' to authenticate\n", os.Args[0])
	}

	return nil
}

func handleConfig() error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}

	path, _ := getConfigPath()
	fmt.Printf("Configuration file: %s\n", path)
	fmt.Printf("Port: %d\n", cfg.Port)
	fmt.Printf("Has GitHub token: %t\n", cfg.GitHubToken != "")
	fmt.Printf("Has Copilot token: %t\n", cfg.CopilotToken != "")
	if cfg.ExpiresAt > 0 {
		fmt.Printf("Token expires at: %d\n", cfg.ExpiresAt)
	}

	return nil
}

func getCurrentTime() int64 {
	return time.Now().Unix()
}

func handleRun() error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}

	// Initialize timeout configurations before any HTTP operations
	initializeTimeouts(cfg)

	// Ensure we're authenticated
	if err := ensureValidToken(cfg); err != nil {
		return fmt.Errorf("authentication failed: %v", err)
	}

	setupLogging()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", modelsHandler(cfg))
	mux.HandleFunc("/v1/chat/completions", proxyHandler(cfg))
	mux.HandleFunc("/health", healthHandler)
	// Add pprof endpoints for profiling
	mux.HandleFunc("/debug/pprof/", http.DefaultServeMux.ServeHTTP)
	mux.HandleFunc("/debug/pprof/cmdline", http.DefaultServeMux.ServeHTTP)
	mux.HandleFunc("/debug/pprof/profile", http.DefaultServeMux.ServeHTTP)
	mux.HandleFunc("/debug/pprof/symbol", http.DefaultServeMux.ServeHTTP)
	mux.HandleFunc("/debug/pprof/trace", http.DefaultServeMux.ServeHTTP)

	port := cfg.Port
	if port == 0 {
		port = 8081
	}

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      mux,
		ReadTimeout:  time.Duration(cfg.Timeouts.ServerRead) * time.Second,
		WriteTimeout: time.Duration(cfg.Timeouts.ServerWrite) * time.Second,
		IdleTimeout:  time.Duration(cfg.Timeouts.ServerIdle) * time.Second,
	}

	setupGracefulShutdown(server)

	fmt.Printf("Starting GitHub Copilot proxy server on port %d...\n", port)
	fmt.Printf("Endpoints:\n")
	fmt.Printf("  - Models: http://localhost:%d/v1/models\n", port)
	fmt.Printf("  - Chat: http://localhost:%d/v1/chat/completions\n", port)
	fmt.Printf("  - Health: http://localhost:%d/health\n", port)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server failed: %v", err)
	}

	return nil
}

func handleModels() error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}

	// Initialize timeout configurations before any HTTP operations
	initializeTimeouts(cfg)

	// Ensure we're authenticated
	if err := ensureValidToken(cfg); err != nil {
		return fmt.Errorf("authentication failed: %v", err)
	}

	// Fetch models
	models, err := fetchModelsFromModelsDev()
	if err != nil {
		fmt.Printf("Failed to fetch models from models.dev: %v\n", err)
		fmt.Println("Using default models:")
		defaultModels := getDefaultModels()
		for _, model := range defaultModels {
			fmt.Printf("  - %s (%s)\n", model.ID, model.OwnedBy)
		}
		return nil
	}

	fmt.Printf("Available models (%d total):\n", len(models.Data))
	for _, model := range models.Data {
		fmt.Printf("  - %s (%s)\n", model.ID, model.OwnedBy)
	}

	return nil
}

func handleRefresh() error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}

	// Initialize timeout configurations before any HTTP operations
	initializeTimeouts(cfg)

	if cfg.CopilotToken == "" {
		return fmt.Errorf("no token to refresh - run 'auth' command first")
	}

	fmt.Println("Forcing token refresh...")
	if err := refreshToken(cfg); err != nil {
		return fmt.Errorf("token refresh failed: %v", err)
	}

	fmt.Printf("✅ Token refresh successful!\n")

	// Show new expiration time
	now := getCurrentTime()
	timeUntilExpiry := cfg.ExpiresAt - now
	minutes := timeUntilExpiry / 60
	seconds := timeUntilExpiry % 60
	fmt.Printf("New token expires in: %dm %ds\n", minutes, seconds)

	return nil
}
