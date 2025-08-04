package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

const (
	copilotAPIBase = "https://api.githubcopilot.com"
	
	// Retry configuration for chat completions
	maxChatRetries = 3
	baseChatRetryDelay = 1 // seconds
)

var tokenMu sync.Mutex

func ensureValidToken(cfg *Config) error {
	tokenMu.Lock()
	defer tokenMu.Unlock()
	
	now := time.Now().Unix()
	
	// Check if token is completely missing
	if cfg.CopilotToken == "" {
		log.Printf("No Copilot token found, starting authentication")
		return authenticate(cfg)
	}
	
	// Proactive refresh: refresh when 20% of lifetime remains or <5 minutes
	timeUntilExpiry := cfg.ExpiresAt - now
	refreshThreshold := int64(300) // 5 minutes
	if cfg.RefreshIn > 0 {
		// Use 20% of RefreshIn as threshold, but minimum 5 minutes
		proactiveThreshold := cfg.RefreshIn / 5 // 20% = 1/5
		if proactiveThreshold > refreshThreshold {
			refreshThreshold = proactiveThreshold
		}
	}
	
	if timeUntilExpiry <= refreshThreshold {
		log.Printf("Token expires in %d seconds (threshold: %d), attempting refresh", timeUntilExpiry, refreshThreshold)
		if err := refreshToken(cfg); err != nil {
			log.Printf("Token refresh failed, falling back to full authentication: %v", err)
			return authenticate(cfg)
		}
		log.Printf("Token refresh completed successfully")
	} else {
		log.Printf("Token is valid: expires in %d seconds", timeUntilExpiry)
	}
	
	return nil
}

// isRetriableError determines if an HTTP error should be retried
func isRetriableError(statusCode int, err error) bool {
	if err != nil {
		return true // Network errors are retriable
	}
	
	// Retry on server errors and rate limiting
	return statusCode >= 500 || statusCode == 429 || statusCode == 408
}

// makeRequestWithRetry performs HTTP request with exponential backoff retry
func makeRequestWithRetry(client *http.Client, req *http.Request, body []byte) (*http.Response, error) {
	var lastResp *http.Response
	var lastErr error
	
	for attempt := 1; attempt <= maxChatRetries; attempt++ {
		// Create a new request for each attempt (in case body was consumed)
		retryReq, err := http.NewRequest(req.Method, req.URL.String(), bytes.NewBuffer(body))
		if err != nil {
			return nil, err
		}
		
		// Copy all headers
		for key, values := range req.Header {
			for _, value := range values {
				retryReq.Header.Add(key, value)
			}
		}
		
		log.Printf("Chat completion attempt %d/%d", attempt, maxChatRetries)
		
		resp, err := client.Do(retryReq)
		if err != nil {
			lastErr = err
			if attempt == maxChatRetries {
				log.Printf("Request failed after %d attempts: %v", maxChatRetries, err)
				return nil, err
			}
			
			waitTime := time.Duration(baseChatRetryDelay*attempt*attempt) * time.Second
			log.Printf("Request failed (attempt %d), retrying in %v: %v", attempt, waitTime, err)
			time.Sleep(waitTime)
			continue
		}
		
		lastResp = resp
		
		// Check if we should retry based on status code
		if !isRetriableError(resp.StatusCode, nil) {
			log.Printf("Request successful on attempt %d: %d", attempt, resp.StatusCode)
			return resp, nil
		}
		
		// Close the response body before retrying
		resp.Body.Close()
		
		if attempt == maxChatRetries {
			log.Printf("Request failed after %d attempts with status: %d", maxChatRetries, resp.StatusCode)
			return resp, nil // Return the last response even if it failed
		}
		
		waitTime := time.Duration(baseChatRetryDelay*attempt*attempt) * time.Second
		log.Printf("Request failed with status %d (attempt %d), retrying in %v", resp.StatusCode, attempt, waitTime)
		time.Sleep(waitTime)
	}
	
	return lastResp, lastErr
}

func proxyHandler(cfg *Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := ensureValidToken(cfg); err != nil {
			log.Printf("Token validation failed: %v", err)
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}

		// Log request
		log.Printf("Request: %s %s", r.Method, r.URL.Path)
		log.Printf("Request Content-Length: %d", r.ContentLength)

		// Read the request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("Error reading request body: %v", err)
			http.Error(w, "Error reading request", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		// Transform path
		targetPath := "/chat/completions"
		if r.URL.Path == "/v1/chat/completions" {
			targetPath = "/chat/completions"
		}

		// Create new request to GitHub Copilot
		targetURL := copilotAPIBase + targetPath
		log.Printf("Sending to: %s", targetURL)
		log.Printf("Request body length: %d", len(body))

		req, err := http.NewRequest(r.Method, targetURL, bytes.NewBuffer(body))
		if err != nil {
			log.Printf("Error creating request: %v", err)
			http.Error(w, "Error creating request", http.StatusInternalServerError)
			return
		}

		// Set headers exactly as the working direct approach
		req.Header.Set("Authorization", "Bearer "+cfg.CopilotToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "GitHubCopilotChat/0.26.7")
		req.Header.Set("Editor-Version", "vscode/1.99.3")
		req.Header.Set("Editor-Plugin-Version", "copilot-chat/0.26.7")
		req.Header.Set("Copilot-Integration-Id", "vscode-chat")
		req.Header.Set("Openai-Intent", "conversation-edits")
		req.Header.Set("X-Initiator", "user")

		// Make the request with retry logic
		client := &http.Client{
			Timeout: 30 * time.Second, // Add timeout
		}
		
		resp, err := makeRequestWithRetry(client, req, body)
		if err != nil {
			log.Printf("Error making request after retries: %v", err)
			http.Error(w, "Error making request", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		log.Printf("Response: %d - Content-Type: %s", resp.StatusCode, resp.Header.Get("Content-Type"))

		// Copy response headers
		for key, values := range resp.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}

		// Add CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "*")

		// Copy status code
		w.WriteHeader(resp.StatusCode)

		// Copy response body
		_, err = io.Copy(w, resp.Body)
		if err != nil {
			log.Printf("Error copying response: %v", err)
		}
	}
}
