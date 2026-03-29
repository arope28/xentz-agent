package config

import (
	"errors"
	"fmt"
	"strings"

	"xentz-agent/internal/secretstore"
)

func deviceAPIKeyStoreKey(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "user":
		return secretstore.KeyDeviceAPIKey + "_user"
	case "system":
		return secretstore.KeyDeviceAPIKey + "_system"
	default:
		return secretstore.KeyDeviceAPIKey
	}
}

func getDeviceAPIKeyForMode(cfg Config, mode string, migrate bool) (string, error) {
	modeKey := deviceAPIKeyStoreKey(mode)
	key, err := secretstore.Get(modeKey)
	if err == nil {
		return strings.TrimSpace(string(key)), nil
	}
	if !errors.Is(err, secretstore.ErrNotFound) {
		return "", fmt.Errorf("secretstore get device api key (%s): %w", modeKey, err)
	}

	// Backward compatibility: read legacy unscoped key and migrate to mode-scoped key.
	legacy, legacyErr := secretstore.Get(secretstore.KeyDeviceAPIKey)
	if legacyErr == nil {
		trimmed := strings.TrimSpace(string(legacy))
		if trimmed != "" {
			if migrate {
				_ = secretstore.Put(modeKey, []byte(trimmed))
			}
			return trimmed, nil
		}
	}
	if legacyErr != nil && !errors.Is(legacyErr, secretstore.ErrNotFound) {
		return "", fmt.Errorf("secretstore get legacy device api key: %w", legacyErr)
	}

	if cfg.DeviceAPIKey != "" {
		_ = secretstore.Put(modeKey, []byte(cfg.DeviceAPIKey))
		return cfg.DeviceAPIKey, nil
	}
	return "", secretstore.ErrNotFound
}

func GetDeviceAPIKeyForMode(cfg Config, mode string) (string, error) {
	return getDeviceAPIKeyForMode(cfg, mode, true)
}

// GetDeviceAPIKeyForModeReadOnly resolves mode+legacy keys without writing/migrating state.
func GetDeviceAPIKeyForModeReadOnly(cfg Config, mode string) (string, error) {
	return getDeviceAPIKeyForMode(cfg, mode, false)
}

func GetDeviceAPIKey(cfg Config) (string, error) {
	return GetDeviceAPIKeyForMode(cfg, cfg.Mode)
}

func StoreDeviceAPIKeyForMode(apiKey, mode string) error {
	if apiKey == "" {
		return fmt.Errorf("device api key is empty")
	}
	return secretstore.Put(deviceAPIKeyStoreKey(mode), []byte(apiKey))
}

func StoreDeviceAPIKey(apiKey string) error {
	return StoreDeviceAPIKeyForMode(apiKey, "")
}

func DeleteDeviceAPIKeyForMode(mode string) error {
	err := secretstore.Delete(deviceAPIKeyStoreKey(mode))
	if err == nil {
		return nil
	}
	if errors.Is(err, secretstore.ErrNotFound) {
		return nil
	}
	return err
}

// DeleteDeviceAPIKeysForMode removes both mode-scoped and legacy keys.
func DeleteDeviceAPIKeysForMode(mode string) error {
	var errs []string
	if err := DeleteDeviceAPIKeyForMode(mode); err != nil {
		errs = append(errs, err.Error())
	}
	if err := secretstore.Delete(secretstore.KeyDeviceAPIKey); err != nil && !errors.Is(err, secretstore.ErrNotFound) {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func GetResticPassword(cfg Config) (string, error) {
	pw, err := secretstore.Get(secretstore.KeyResticPassword)
	if err == nil {
		return strings.TrimSpace(string(pw)), nil
	}
	if !errors.Is(err, secretstore.ErrNotFound) {
		return "", fmt.Errorf("secretstore get restic password: %w", err)
	}
	return "", secretstore.ErrNotFound
}

func StoreResticPassword(password string) error {
	if password == "" {
		return fmt.Errorf("restic password is empty")
	}
	return secretstore.Put(secretstore.KeyResticPassword, []byte(password))
}
