package config

import (
	"os"
	"testing"
	"time"
)

func TestRefreshIntervalFromEnv(t *testing.T) {
	t.Cleanup(func() { _ = os.Unsetenv("XENTZ_CONFIG_REFRESH_INTERVAL") })

	_ = os.Unsetenv("XENTZ_CONFIG_REFRESH_INTERVAL")
	if d := RefreshIntervalFromEnv(); d != 5*time.Minute {
		t.Fatalf("default: got %v", d)
	}

	_ = os.Setenv("XENTZ_CONFIG_REFRESH_INTERVAL", "10m")
	if d := RefreshIntervalFromEnv(); d != 10*time.Minute {
		t.Fatalf("10m: got %v", d)
	}

	_ = os.Setenv("XENTZ_CONFIG_REFRESH_INTERVAL", "off")
	if d := RefreshIntervalFromEnv(); d != 0 {
		t.Fatalf("off: got %v", d)
	}
}
