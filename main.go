package main

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"time"
)

// version will be set by the build process
var version = "dev"

type Config struct {
	Port         int    `json:"port"`
	GitHubToken  string `json:"github_token"`
	CopilotToken string `json:"copilot_token"`
	ExpiresAt    int64  `json:"expires_at"`
	RefreshIn    int64  `json:"refresh_in"`

	// Timeout configurations (in seconds)
	Timeouts struct {
		HTTPClient      int `json:"http_client"`       // Default: 300s for streaming responses
		ServerRead      int `json:"server_read"`       // Default: 30s for request reading
		ServerWrite     int `json:"server_write"`      // Default: 300s for streaming responses
		ServerIdle      int `json:"server_idle"`       // Default: 120s for idle connections
		ProxyContext    int `json:"proxy_context"`     // Default: 300s for proxy request context
		CircuitBreaker  int `json:"circuit_breaker"`   // Default: 30s for circuit breaker recovery
		KeepAlive       int `json:"keep_alive"`        // Default: 30s for connection keep-alive
		TLSHandshake    int `json:"tls_handshake"`     // Default: 10s for TLS handshake
		DialTimeout     int `json:"dial_timeout"`      // Default: 10s for connection dialing
		IdleConnTimeout int `json:"idle_conn_timeout"` // Default: 90s for idle connection timeout
	} `json:"timeouts"`
}

const (
	configDirName  = ".local/share/github-copilot-svcs"
	configFileName = "config.json"
)

func getConfigPath() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(usr.HomeDir, configDirName)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

func getDefaultModels() []Model {
	// Models based on actual models.dev GitHub Copilot, Claude, and Gemini entries (as of August 2025)
	return []Model{
		// GitHub Copilot (OpenAI-compatible)
		{ID: "gpt-4o", Object: "model", Created: time.Now().Unix(), OwnedBy: "openai"},
		{ID: "gpt-4.1", Object: "model", Created: time.Now().Unix(), OwnedBy: "openai"},
		{ID: "o3", Object: "model", Created: time.Now().Unix(), OwnedBy: "openai"},
		{ID: "o3-mini", Object: "model", Created: time.Now().Unix(), OwnedBy: "openai"},
		{ID: "o4-mini", Object: "model", Created: time.Now().Unix(), OwnedBy: "openai"},
		// Claude (Anthropic)
		{ID: "claude-3.5-sonnet", Object: "model", Created: time.Now().Unix(), OwnedBy: "anthropic"},
		{ID: "claude-3.7-sonnet", Object: "model", Created: time.Now().Unix(), OwnedBy: "anthropic"},
		{ID: "claude-3.7-sonnet-thought", Object: "model", Created: time.Now().Unix(), OwnedBy: "anthropic"},
		{ID: "claude-opus-4", Object: "model", Created: time.Now().Unix(), OwnedBy: "anthropic"},
		{ID: "claude-sonnet-4", Object: "model", Created: time.Now().Unix(), OwnedBy: "anthropic"},
		// Gemini (Google)
		{ID: "gemini-2.5-pro", Object: "model", Created: time.Now().Unix(), OwnedBy: "google"},
		{ID: "gemini-2.0-flash-001", Object: "model", Created: time.Now().Unix(), OwnedBy: "google"},
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: github-copilot-svcs <command>")
		fmt.Println("Commands:")
		fmt.Println("  auth     Authenticate with GitHub Copilot")
		fmt.Println("  run      Start the proxy server")
		fmt.Println("  models   List available models")
		fmt.Println("  config   Show current configuration")
		fmt.Println("  status   Show authentication and token status")
		fmt.Println("  refresh  Force refresh of Copilot token")
		fmt.Println("  version  Show version information")
		return
	}

	switch os.Args[1] {
	case "auth":
		if err := handleAuth(); err != nil {
			fmt.Printf("Authentication failed: %v\n", err)
			os.Exit(1)
		}
	case "run":
		if err := handleRun(); err != nil {
			fmt.Printf("Server failed: %v\n", err)
			os.Exit(1)
		}
	case "models":
		if err := handleModels(); err != nil {
			fmt.Printf("Models command failed: %v\n", err)
			os.Exit(1)
		}
	case "config":
		if err := handleConfig(); err != nil {
			fmt.Printf("Config command failed: %v\n", err)
			os.Exit(1)
		}
	case "status":
		if err := handleStatus(); err != nil {
			fmt.Printf("Status command failed: %v\n", err)
			os.Exit(1)
		}
	case "refresh":
		if err := handleRefresh(); err != nil {
			fmt.Printf("Refresh command failed: %v\n", err)
			os.Exit(1)
		}
	case "version":
		fmt.Printf("github-copilot-svcs version %s\n", version)
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}
