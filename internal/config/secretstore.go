package config

import (
	"errors"
	"fmt"
	"strings"

	"xentz-agent/internal/secretstore"
)

func GetDeviceAPIKey(cfg Config) (string, error) {
	key, err := secretstore.Get(secretstore.KeyDeviceAPIKey)
	if err == nil {
		return strings.TrimSpace(string(key)), nil
	}
	if !errors.Is(err, secretstore.ErrNotFound) {
		return "", fmt.Errorf("secretstore get device api key: %w", err)
	}

	if cfg.DeviceAPIKey != "" {
		_ = secretstore.Put(secretstore.KeyDeviceAPIKey, []byte(cfg.DeviceAPIKey))
		return cfg.DeviceAPIKey, nil
	}
	return "", secretstore.ErrNotFound
}

func StoreDeviceAPIKey(apiKey string) error {
	if apiKey == "" {
		return fmt.Errorf("device api key is empty")
	}
	return secretstore.Put(secretstore.KeyDeviceAPIKey, []byte(apiKey))
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
