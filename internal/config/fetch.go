package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// FetchFromServer fetches configuration from the server using the device API key
func FetchFromServer(serverURL, deviceAPIKey string) (Config, error) {
	if serverURL == "" {
		return Config{}, fmt.Errorf("server URL is required")
	}
	if deviceAPIKey == "" {
		return Config{}, fmt.Errorf("device API key is required")
	}

	// Make GET request to /v1/config
	url := fmt.Sprintf("%s/v1/config", serverURL)
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
		errMsg.ReadFrom(resp.Body)
		return Config{}, fmt.Errorf("config fetch failed (status %d): %s", resp.StatusCode, errMsg.String())
	}

	// Parse response
	var cfg Config
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config response: %w", err)
	}

	// Validate required fields
	if len(cfg.Include) == 0 {
		return Config{}, fmt.Errorf("server config missing required field: include")
	}
	if cfg.Restic.Repository == "" {
		return Config{}, fmt.Errorf("server config missing required field: restic.repository")
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
func LoadWithFallback(serverURL, deviceAPIKey string) (Config, error) {
	// Try to fetch from server
	cfg, err := FetchAndCache(serverURL, deviceAPIKey)
	if err == nil {
		log.Println("✓ Config fetched from server and cached")
		return cfg, nil
	}

	// Fetch failed, try to load cached config
	log.Printf("warning: failed to fetch config from server: %v", err)
	log.Println("Attempting to use cached config...")

	cachedCfg, cacheErr := ReadCached()
	if cacheErr != nil {
		return Config{}, fmt.Errorf("config fetch failed and no cached config available: %w (cache error: %v)", err, cacheErr)
	}

	log.Println("⚠ Using cached config (server unreachable or config fetch failed)")
	return cachedCfg, nil
}

