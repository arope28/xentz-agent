package config

import (
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"time"

	"xentz-agent/internal/identity"
	"xentz-agent/internal/secretstore"
)

// RefreshIntervalFromEnv returns the ticker interval for periodic config refresh.
// XENTZ_CONFIG_REFRESH_INTERVAL: a Go duration string (e.g. 5m, 15m). If unset, defaults to 5m.
// Use 0, off, or disabled to turn auto-refresh off (callers should treat d <= 0 as off).
func RefreshIntervalFromEnv() time.Duration {
	s := strings.TrimSpace(os.Getenv("XENTZ_CONFIG_REFRESH_INTERVAL"))
	if s == "" {
		return 5 * time.Minute
	}
	if s == "0" || strings.EqualFold(s, "off") || strings.EqualFold(s, "disabled") {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 5 * time.Minute
	}
	return d
}

// StartAutoRefresh periodically fetches config and caches it.
// Callers should cancel the context to stop the refresh loop.
func StartAutoRefresh(ctx context.Context, interval time.Duration, serverURL, deviceAPIKey string) {
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := LoadWithFallback(serverURL, deviceAPIKey); err != nil {
					log.Printf("config auto-refresh failed: %v", err)
				}
			}
		}
	}()
}

// StartAutoRefreshForConfigFile starts StartAutoRefresh for an enrolled device using cfgPath (config.json)
// and the device API key from the secret store. mode is used to load identity.json when server_url is missing.
func StartAutoRefreshForConfigFile(ctx context.Context, cfgPath string, mode string) {
	interval := RefreshIntervalFromEnv()
	if interval <= 0 {
		return
	}
	cfg, err := Read(cfgPath)
	if err != nil {
		return
	}
	if strings.TrimSpace(mode) == "" {
		mode = strings.TrimSpace(cfg.Mode)
	}
	if strings.TrimSpace(mode) == "" {
		mode = "user"
	}

	serverURL := strings.TrimSpace(cfg.ServerURL)
	if serverURL == "" {
		if id, err := identity.Load(mode); err == nil {
			serverURL = strings.TrimSpace(id.ServerURL)
		}
	}
	apiKey, err := GetDeviceAPIKey(cfg)
	if err != nil {
		if !errors.Is(err, secretstore.ErrNotFound) {
			log.Printf("config auto-refresh: device api key: %v", err)
		}
		return
	}
	if serverURL == "" || apiKey == "" {
		return
	}
	log.Printf("config auto-refresh enabled (every %s)", interval)
	StartAutoRefresh(ctx, interval, serverURL, apiKey)
}
