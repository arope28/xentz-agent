//go:build darwin

package secretstore

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

const keychainService = "com.xentz.agent"

type keychainStore struct{}

func newStore() Store {
	return &keychainStore{}
}

func (s *keychainStore) Get(key string) ([]byte, error) {
	cmd := exec.Command("security", "find-generic-password", "-s", keychainService, "-a", key, "-w")
	out, err := cmd.Output()
	if err != nil {
		if isKeychainNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("keychain get: %w", err)
	}
	return bytes.TrimSpace(out), nil
}

func (s *keychainStore) Put(key string, value []byte) error {
	cmd := exec.Command("security", "add-generic-password", "-U", "-s", keychainService, "-a", key, "-w", string(value))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("keychain put: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (s *keychainStore) Delete(key string) error {
	cmd := exec.Command("security", "delete-generic-password", "-s", keychainService, "-a", key)
	if out, err := cmd.CombinedOutput(); err != nil {
		if isKeychainNotFound(err) {
			return ErrNotFound
		}
		return fmt.Errorf("keychain delete: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func isKeychainNotFound(err error) bool {
	if err == nil {
		return false
	}
	// security returns exit code 44 for item not found, but we only have stderr text
	return strings.Contains(err.Error(), "could not be found") || strings.Contains(err.Error(), "SecKeychainSearchCopyNext")
}
