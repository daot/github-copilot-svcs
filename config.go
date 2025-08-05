package main

import (
	"encoding/json"
	"os"
)

func loadConfig() (*Config, error) {
	path, err := getConfigPath()
	if err != nil {
		return nil, err
	}

	file, err := os.Open(path)
	if err != nil {
		// Return default config if file doesn't exist
		cfg := &Config{Port: 8081}
		setDefaultTimeouts(cfg)
		return cfg, nil
	}
	defer file.Close()

	var cfg Config
	if err := json.NewDecoder(file).Decode(&cfg); err != nil {
		return nil, err
	}

	// Set default port if not specified
	if cfg.Port == 0 {
		cfg.Port = 8081
	}

	// Set default timeouts if not specified
	setDefaultTimeouts(&cfg)

	return &cfg, nil
}

// setDefaultTimeouts sets default timeout values if they are zero
func setDefaultTimeouts(cfg *Config) {
	if cfg.Timeouts.HTTPClient == 0 {
		cfg.Timeouts.HTTPClient = 300
	}
	if cfg.Timeouts.ServerRead == 0 {
		cfg.Timeouts.ServerRead = 30
	}
	if cfg.Timeouts.ServerWrite == 0 {
		cfg.Timeouts.ServerWrite = 300
	}
	if cfg.Timeouts.ServerIdle == 0 {
		cfg.Timeouts.ServerIdle = 120
	}
	if cfg.Timeouts.ProxyContext == 0 {
		cfg.Timeouts.ProxyContext = 300
	}
	if cfg.Timeouts.CircuitBreaker == 0 {
		cfg.Timeouts.CircuitBreaker = 30
	}
	if cfg.Timeouts.KeepAlive == 0 {
		cfg.Timeouts.KeepAlive = 30
	}
	if cfg.Timeouts.TLSHandshake == 0 {
		cfg.Timeouts.TLSHandshake = 10
	}
	if cfg.Timeouts.DialTimeout == 0 {
		cfg.Timeouts.DialTimeout = 10
	}
	if cfg.Timeouts.IdleConnTimeout == 0 {
		cfg.Timeouts.IdleConnTimeout = 90
	}
}

func saveConfig(cfg *Config) error {
	path, err := getConfigPath()
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(cfg)
}
