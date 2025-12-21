package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Schedule struct {
	// MVP: daily at HH:MM local time (launchd handles scheduling)
	DailyAt string `json:"daily_at"`
}
type Restic struct {
	Repository   string `json:"repository"`              // e.g. "rest:https://.../restic/dr-core-backups-demo/client-123/"
	PasswordFile string `json:"password_file,omitempty"` // e.g. "~/.xentz-agent/restic.pw"
}

type Retention struct {
	KeepLast    int `json:"keep_last,omitempty"`
	KeepDaily   int `json:"keep_daily,omitempty"`
	KeepWeekly  int `json:"keep_weekly,omitempty"`
	KeepMonthly int `json:"keep_monthly,omitempty"`
	KeepYearly  int `json:"keep_yearly,omitempty"`

	// Prune policy
	Prune bool `json:"prune"` // recommended true
}

type Config struct {
	// Enrollment fields (server-issued identifiers)
	InstallToken string `json:"install_token,omitempty"` // Temporary token for enrollment (not stored after enrollment)
	TenantID     string `json:"tenant_id,omitempty"`     // Server-assigned tenant/customer ID
	DeviceID     string `json:"device_id,omitempty"`     // Server-assigned device identifier
	DeviceAPIKey string `json:"device_api_key,omitempty"` // Long-lived API key for fetching config
	UserID       string `json:"user_id,omitempty"`       // User identifier (username or UUID)

	// Control plane and scheduling
	ServerURL string   `json:"server_url,omitempty"` // Base URL for control plane
	Schedule  Schedule `json:"schedule"`
	Include   []string `json:"include"`
	Exclude   []string `json:"exclude,omitempty"`
	Restic    Restic   `json:"restic"`
	Retention Retention `json:"retention,omitempty"`
}

func ResolvePath(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".xentz-agent", "config.json"), nil
}

func EnsureDirFor(path string) error {
	dir := filepath.Dir(path)
	return os.MkdirAll(dir, 0o700)
}

func Write(path string, cfg Config) error {
	if err := EnsureDirFor(path); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func Read(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// GetCachedConfigPath returns the path for the cached config file
func GetCachedConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".xentz-agent", "config-cached.json"), nil
}

// WriteCached writes the config to the cached config file
func WriteCached(cfg Config) error {
	cachePath, err := GetCachedConfigPath()
	if err != nil {
		return err
	}
	return Write(cachePath, cfg)
}

// ReadCached reads the cached config file
func ReadCached() (Config, error) {
	cachePath, err := GetCachedConfigPath()
	if err != nil {
		return Config{}, err
	}
	return Read(cachePath)
}
