package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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

	// Ensure identity fields are present for scoping
	if strings.TrimSpace(cfg.TenantID) == "" || strings.TrimSpace(cfg.DeviceID) == "" {
		return Config{}, fmt.Errorf("server config missing required identity fields (tenant_id/device_id)")
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

// FetchPassword retrieves the restic password from the server
// This is used to recover the password if the local password file is lost
func FetchPassword(serverURL, deviceAPIKey string) (string, error) {
	if serverURL == "" {
		return "", fmt.Errorf("server URL is required")
	}
	if deviceAPIKey == "" {
		return "", fmt.Errorf("device API key is required")
	}

	// Validate server URL to prevent SSRF
	if err := validation.ValidateServerURL(serverURL); err != nil {
		return "", fmt.Errorf("invalid server URL: %w", err)
	}

	// Make GET request to /control/v1/password
	url := fmt.Sprintf("%s/control/v1/password", serverURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", deviceAPIKey))
	req.Header.Set("Accept", "application/json")

	// Set timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("password fetch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		var errMsg bytes.Buffer
		errMsg.ReadFrom(resp.Body)
		return "", fmt.Errorf("authentication failed (status %d): invalid or revoked device API key", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		var errMsg bytes.Buffer
		io.CopyN(&errMsg, resp.Body, 512)
		errStr := strings.TrimSpace(errMsg.String())
		return "", fmt.Errorf("password fetch failed (status %d): %s", resp.StatusCode, errStr)
	}

	// Parse response
	var passwordResp struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&passwordResp); err != nil {
		return "", fmt.Errorf("decode password response: %w", err)
	}

	if passwordResp.Password == "" {
		return "", fmt.Errorf("server returned empty password")
	}

	return passwordResp.Password, nil
}

// UpdateConfigOnServer updates configuration on the server using the device API key
func UpdateConfigOnServer(serverURL, deviceAPIKey string, include, exclude []string) (Config, error) {
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

	// Prepare request body
	reqBody := map[string]interface{}{
		"include": include,
		"exclude": exclude,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return Config{}, fmt.Errorf("marshal request body: %w", err)
	}

	// Make PUT request to /control/v1/config
	url := fmt.Sprintf("%s/control/v1/config", serverURL)
	req, err := http.NewRequest("PUT", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return Config{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", deviceAPIKey))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Set timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return Config{}, fmt.Errorf("config update failed: %w", err)
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
		return Config{}, fmt.Errorf("config update failed (status %d): %s", resp.StatusCode, errStr)
	}

	// Parse response
	var cfg Config
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config response: %w", err)
	}

	// KILL-SWITCH: Check if device is disabled (enabled=false)
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

	return cfg, nil
}

// EnsurePasswordFile ensures the restic password is available in secretstore.
// It migrates from a legacy password file if present, or fetches from server.
func EnsurePasswordFile(cfg Config, serverURL, deviceAPIKey string) error {
	// 1) If secretstore already has the password, we are done.
	if pw, err := GetResticPassword(cfg); err == nil && strings.TrimSpace(pw) != "" {
		if cfg.Restic.Repository != "" {
			if err := validatePasswordWithSecret(cfg.Restic.Repository, pw); err != nil {
				log.Printf("warning: stored password validation failed: %v", err)
			}
		}
		return nil
	}

	// 2) If a legacy password file is configured, migrate it into secretstore.
	passwordPath := strings.TrimSpace(cfg.Restic.PasswordFile)
	if passwordPath != "" {
		if strings.HasPrefix(passwordPath, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("get home directory: %w", err)
			}
			passwordPath = filepath.Join(home, strings.TrimPrefix(passwordPath, "~/"))
		}

		if data, err := os.ReadFile(passwordPath); err == nil {
			pw := strings.TrimSpace(string(data))
			if pw != "" {
				if err := StoreResticPassword(pw); err != nil {
					return fmt.Errorf("store restic password: %w", err)
				}
				if cfg.Restic.Repository != "" {
					if err := validatePasswordWithSecret(cfg.Restic.Repository, pw); err != nil {
						log.Printf("warning: migrated password validation failed: %v", err)
					}
				}
				return nil
			}
		}
	}

	// 3) Fetch from server if possible
	if serverURL == "" || deviceAPIKey == "" {
		return fmt.Errorf("restic password missing and cannot recover (server URL or API key not available)")
	}
	log.Println("Restic password missing, attempting to recover from server...")
	password, err := FetchPassword(serverURL, deviceAPIKey)
	if err != nil {
		return fmt.Errorf("failed to retrieve password from server: %w", err)
	}
	if err := StoreResticPassword(password); err != nil {
		return fmt.Errorf("failed to store password: %w", err)
	}
	log.Println("✓ Password recovered from server and saved to secretstore")

	if cfg.Restic.Repository != "" {
		if err := validatePasswordWithSecret(cfg.Restic.Repository, password); err != nil {
			log.Printf("warning: recovered password validation failed: %v", err)
		}
	}
	return nil
}

// validatePassword tests if the password file contains the correct password for the repository
// by attempting to read the repository config
func validatePassword(repository, passwordFile string) error {
	// Use "restic cat config" to test if password is correct
	// This command will fail with "wrong password" if the password is incorrect
	cmd := exec.Command("restic", "cat", "config")
	cmd.Env = append(os.Environ(),
		"RESTIC_REPOSITORY="+repository,
		"RESTIC_PASSWORD_FILE="+expandHomePath(passwordFile),
	)

	// Capture output to check for password errors
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		errStr := out.String()
		// Check if error is due to wrong password
		if strings.Contains(errStr, "wrong password") || strings.Contains(errStr, "no key found") {
			return fmt.Errorf("wrong password: %w", err)
		}
		// Other errors (network, repo doesn't exist) are not password validation failures
		// We'll let those pass through - they'll be caught during actual backup
	}

	return nil
}

// validatePasswordWithSecret tests if the password is correct by using RESTIC_PASSWORD env.
func validatePasswordWithSecret(repository, password string) error {
	cmd := exec.Command("restic", "cat", "config")
	cmd.Env = append(os.Environ(),
		"RESTIC_REPOSITORY="+repository,
		"RESTIC_PASSWORD="+password,
	)

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		errStr := out.String()
		if strings.Contains(errStr, "wrong password") || strings.Contains(errStr, "no key found") {
			return fmt.Errorf("wrong password: %w", err)
		}
	}

	return nil
}

// expandHomePath expands ~ to home directory
func expandHomePath(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p // Return as-is if we can't get home dir
		}
		return filepath.Join(home, strings.TrimPrefix(p, "~/"))
	}
	return p
}
