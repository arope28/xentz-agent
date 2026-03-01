package config

import (
	"context"
	"log"
	"time"
)

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
