//go:build linux

package secretstore

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

type libsecretStore struct{}

func newStore() Store {
	if _, err := exec.LookPath("secret-tool"); err == nil {
		return &libsecretStore{}
	}
	return newFileStore()
}

func (s *libsecretStore) Get(key string) ([]byte, error) {
	cmd := exec.Command("secret-tool", "lookup", "xentz-agent", "key", key)
	out, err := cmd.Output()
	if err != nil {
		if isLibsecretNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("libsecret get: %w", err)
	}
	return bytes.TrimSpace(out), nil
}

func (s *libsecretStore) Put(key string, value []byte) error {
	cmd := exec.Command("secret-tool", "store", "--label", "Xentz Agent", "xentz-agent", "key", key)
	cmd.Stdin = bytes.NewReader(value)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("libsecret put: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (s *libsecretStore) Delete(key string) error {
	cmd := exec.Command("secret-tool", "clear", "xentz-agent", "key", key)
	if out, err := cmd.CombinedOutput(); err != nil {
		if isLibsecretNotFound(err) {
			return ErrNotFound
		}
		return fmt.Errorf("libsecret delete: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func isLibsecretNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "No such secret") || strings.Contains(err.Error(), "not found")
}
