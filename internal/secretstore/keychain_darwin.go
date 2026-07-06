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

func newPlatformStore() Store {
	return &keychainStore{}
}

func (s *keychainStore) Get(key string) ([]byte, error) {
	cmd := exec.Command("security", "find-generic-password", "-s", keychainService, "-a", key, "-w")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if isKeychainNotFound(err, out) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("keychain get: %w (output: %s)", err, strings.TrimSpace(string(out)))
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
		if isKeychainNotFound(err, out) {
			return ErrNotFound
		}
		return fmt.Errorf("keychain delete: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func isKeychainNotFound(err error, output []byte) bool {
	if err == nil {
		return false
	}
	// `security` uses exit code 44 for "item not found".
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 44 {
		return true
	}
	msg := strings.ToLower(err.Error() + " " + string(output))
	return strings.Contains(msg, "could not be found") || strings.Contains(msg, "seckeychainsearchcopynext")
}
