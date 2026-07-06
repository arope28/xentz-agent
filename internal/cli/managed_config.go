package cli

import (
	"fmt"
	"log"
	"strings"

	"xentz-agent/internal/config"
	"xentz-agent/internal/state"
)

type managedConfig struct {
	Local     config.Config
	Effective config.Config
}

func loadManagedConfig(command, cfgFile string, markRevoked bool) (managedConfig, error) {
	localCfg, apiKey, err := LoadEnrollment(cfgFile, "")
	if err != nil {
		return managedConfig{}, fmt.Errorf("load enrollment: %w", err)
	}
	localCfg.DeviceAPIKey = apiKey
	if err := ModeMismatchError(command, localCfg); err != nil {
		return managedConfig{}, err
	}

	if localCfg.DeviceAPIKey == "" || localCfg.ServerURL == "" {
		if localCfg.Restic.Repository == "" {
			return managedConfig{}, fmt.Errorf("missing config and no enrollment identity available (run `xentz-agent install` or `xentz-agent recover`)")
		}
		log.Println("Using local config (device not enrolled or legacy mode)")
		if localCfg.Enabled != nil && !*localCfg.Enabled {
			return managedConfig{}, fmt.Errorf("device is disabled by server (kill-switch activated). All operations stopped")
		}
		return managedConfig{Local: localCfg, Effective: localCfg}, nil
	}

	fetchedCfg, fetchErr := config.LoadWithFallback(localCfg.ServerURL, localCfg.DeviceAPIKey)
	if fetchErr != nil {
		if markRevoked && (strings.Contains(fetchErr.Error(), "authentication failed") || strings.Contains(fetchErr.Error(), "revoked")) {
			if st, err := state.New(); err == nil {
				_ = st.SetRevoked(true)
			}
		}
		return managedConfig{}, fmt.Errorf("failed to load config: %w", fetchErr)
	}

	cfg := fetchedCfg
	cfg.TenantID = localCfg.TenantID
	cfg.DeviceID = localCfg.DeviceID
	cfg.DeviceAPIKey = localCfg.DeviceAPIKey
	cfg.ServerURL = localCfg.ServerURL
	cfg.UserID = localCfg.UserID
	cfg.Restic.PasswordFile = localCfg.Restic.PasswordFile

	if err := config.EnsurePasswordFile(cfg, localCfg.ServerURL, localCfg.DeviceAPIKey); err != nil {
		return managedConfig{}, fmt.Errorf("password file validation failed: %w", err)
	}
	if cfg.Enabled != nil && !*cfg.Enabled {
		return managedConfig{}, fmt.Errorf("device is disabled by server (kill-switch activated). All operations stopped")
	}

	return managedConfig{Local: localCfg, Effective: cfg}, nil
}

func loadResticConfigAndPassword(cfgFile string) (config.Config, string, error) {
	managed, err := loadManagedConfig("restore", cfgFile, false)
	if err != nil {
		if strings.Contains(err.Error(), "missing config and no enrollment identity available") {
			return config.Config{}, "", fmt.Errorf("missing config and no enrollment (run `xentz-agent install` or `xentz-agent recover`)")
		}
		return config.Config{}, "", err
	}

	var resticPassword string
	if managed.Effective.Restic.PasswordFile == "" {
		pw, err := config.GetResticPassword(managed.Effective)
		if err != nil {
			return config.Config{}, "", fmt.Errorf("restic password: %w", err)
		}
		resticPassword = pw
	}
	return managed.Effective, resticPassword, nil
}
