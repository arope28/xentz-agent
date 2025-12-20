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

type Config struct {
	ServerURL string   `json:"server_url,omitempty"` // optional; not required for restic REST backend
	Schedule  Schedule `json:"schedule"`
	Include   []string `json:"include"`
	Exclude   []string `json:"exclude,omitempty"`
	Restic    Restic   `json:"restic"`
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