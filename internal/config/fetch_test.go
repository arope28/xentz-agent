package config

import (
	"strings"
	"testing"
)

func boolPtr(b bool) *bool { return &b }

func TestValidateConfigResponseFromServer(t *testing.T) {
	valid := Config{
		TenantID: "t1",
		DeviceID: "d1",
		Include:  []string{"/home/u/docs"},
		Exclude:  []string{"*.tmp"},
		Restic:   Restic{Repository: "rest:https://example.com/r"},
	}

	t.Run("ok", func(t *testing.T) {
		if err := validateConfigResponseFromServer(valid); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("kill_switch", func(t *testing.T) {
		cfg := valid
		cfg.Enabled = boolPtr(false)
		err := validateConfigResponseFromServer(cfg)
		if err == nil || !strings.Contains(err.Error(), "kill-switch") {
			t.Fatalf("expected kill-switch error, got %v", err)
		}
	})

	t.Run("missing_tenant", func(t *testing.T) {
		cfg := valid
		cfg.TenantID = ""
		err := validateConfigResponseFromServer(cfg)
		if err == nil || !strings.Contains(err.Error(), "tenant_id") {
			t.Fatalf("expected identity error, got %v", err)
		}
	})

	t.Run("too_many_include", func(t *testing.T) {
		cfg := valid
		cfg.Include = make([]string, 1001)
		for i := range cfg.Include {
			cfg.Include[i] = "/p"
		}
		err := validateConfigResponseFromServer(cfg)
		if err == nil || !strings.Contains(err.Error(), "too many include") {
			t.Fatalf("expected count error, got %v", err)
		}
	})

	t.Run("null_byte_in_path", func(t *testing.T) {
		cfg := valid
		cfg.Include = []string{"/good", "/bad\x00"}
		err := validateConfigResponseFromServer(cfg)
		if err == nil || !strings.Contains(err.Error(), "null byte") {
			t.Fatalf("expected path error, got %v", err)
		}
	})
}
