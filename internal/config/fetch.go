package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"xentz-agent/internal/validation"
)

// FetchFromServer fetches configuration from the server using the device API key
func FetchFromServer(serverURL, deviceAPIKey string) (Config, error) {
	if serverURL == "" {
		return Config{}, fmt.Errorf("server URL is required")
	}
	if deviceAPIKey == "" {
		return Config{}, fmt.Errorf("device API key is required")
	}

	// Validate server URL to prevent SSRF
	if err := validation.ValidateServerURL(serverURL); err != nil {
		return Config{}, fmt.Errorf("invalid server URL: %w", err)
	}

	// Make GET request to /control/v1/config
	// Note: nginx proxies /control/* to the control plane backend
	url := fmt.Sprintf("%s/control/v1/config", serverURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return Config{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", deviceAPIKey))
	req.Header.Set("Accept", "application/json")

	// Set timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return Config{}, fmt.Errorf("config fetch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		var errMsg bytes.Buffer
		errMsg.ReadFrom(resp.Body)
		return Config{}, fmt.Errorf("authentication failed (status %d): invalid or revoked device API key", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		var errMsg bytes.Buffer
		// Limit error message to prevent information leakage
		io.CopyN(&errMsg, resp.Body, 512) // Limit to 512 bytes
		errStr := strings.TrimSpace(errMsg.String())
		// Remove newlines and limit length
		errStr = strings.ReplaceAll(errStr, "\n", " ")
		errStr = strings.ReplaceAll(errStr, "\r", " ")
		if len(errStr) > 256 {
			errStr = errStr[:256] + "..."
		}
		return Config{}, fmt.Errorf("config fetch failed (status %d): %s", resp.StatusCode, errStr)
	}

	// Parse response
	var cfg Config
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config response: %w", err)
	}

	// KILL-SWITCH: Check if device is disabled (enabled=false)
	// This must be checked BEFORE any other validation to ensure disabled status takes precedence
	if cfg.Enabled != nil && !*cfg.Enabled {
		return Config{}, fmt.Errorf("device is disabled by server (kill-switch activated)")
	}

	// Validate required fields
	if len(cfg.Include) == 0 {
		return Config{}, fmt.Errorf("server config missing required field: include")
	}
	if cfg.Restic.Repository == "" {
		return Config{}, fmt.Errorf("server config missing required field: restic.repository")
	}

	// Validate config values to prevent malicious input
	if len(cfg.Include) > 1000 {
		return Config{}, fmt.Errorf("too many include paths (max 1000)")
	}
	if len(cfg.Exclude) > 1000 {
		return Config{}, fmt.Errorf("too many exclude paths (max 1000)")
	}

	// Validate paths
	validatePath := func(path string) error {
		if len(path) == 0 || len(path) > 4096 {
			return fmt.Errorf("path length invalid")
		}
		if strings.Contains(path, "\x00") {
			return fmt.Errorf("path contains null byte")
		}
		return nil
	}

	for i, path := range cfg.Include {
		if err := validatePath(path); err != nil {
			return Config{}, fmt.Errorf("invalid include path at index %d: %w", i, err)
		}
	}
	for i, path := range cfg.Exclude {
		if err := validatePath(path); err != nil {
			return Config{}, fmt.Errorf("invalid exclude path at index %d: %w", i, err)
		}
	}

	return cfg, nil
}

// FetchAndCache fetches config from server, validates it, and caches it locally
func FetchAndCache(serverURL, deviceAPIKey string) (Config, error) {
	cfg, err := FetchFromServer(serverURL, deviceAPIKey)
	if err != nil {
		return Config{}, err
	}

	// Cache the config
	if err := WriteCached(cfg); err != nil {
		log.Printf("warning: failed to cache config: %v", err)
		// Continue even if caching fails
	}

	return cfg, nil
}

// LoadWithFallback attempts to fetch config from server, falling back to cached config if server is unreachable
// IMPORTANT: If server returns enabled=false (kill-switch), this function will return an error and NOT use cached config.
// This ensures that a disabled device cannot continue operating even with cached config.
func LoadWithFallback(serverURL, deviceAPIKey string) (Config, error) {
	// Try to fetch from server
	cfg, err := FetchAndCache(serverURL, deviceAPIKey)
	if err == nil {
		log.Println("✓ Config fetched from server and cached")
		return cfg, nil
	}

	// Check if the error is due to device being disabled (kill-switch)
	// If so, we MUST NOT use cached config - the device must be disabled
	if strings.Contains(err.Error(), "device is disabled") || strings.Contains(err.Error(), "kill-switch") {
		return Config{}, fmt.Errorf("device is disabled by server: %w", err)
	}

	// Check if the error is due to authentication failure (401/403)
	// This could indicate API key revocation, so we should not use cached config
	if strings.Contains(err.Error(), "authentication failed") || strings.Contains(err.Error(), "invalid or revoked") {
		return Config{}, fmt.Errorf("authentication failed (API key may be revoked): %w", err)
	}

	// For other errors (network issues, etc.), we can fall back to cached config
	log.Printf("warning: failed to fetch config from server: %v", err)
	log.Println("Attempting to use cached config...")

	cachedCfg, cacheErr := ReadCached()
	if cacheErr != nil {
		return Config{}, fmt.Errorf("config fetch failed and no cached config available: %w (cache error: %v)", err, cacheErr)
	}

	// IMPORTANT: Even when using cached config, check if it was previously disabled
	// This prevents a device from continuing if it was disabled before going offline
	if cachedCfg.Enabled != nil && !*cachedCfg.Enabled {
		return Config{}, fmt.Errorf("device is disabled (cached config shows enabled=false)")
	}

	log.Println("⚠ Using cached config (server unreachable or config fetch failed)")
	return cachedCfg, nil
}
