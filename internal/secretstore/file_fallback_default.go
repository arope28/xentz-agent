//go:build !darwin && !windows && !linux

package secretstore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"xentz-agent/internal/paths"
)

// defaultStore is used on FreeBSD and other Unix-like systems that have no
// platform-specific secret store (keychain, DPAPI, libsecret).
type defaultFileStore struct {
	dir string
}

func newStore() Store {
	dir, err := paths.ConfigDir("")
	if err != nil {
		return &defaultFileStore{dir: ""}
	}
	return &defaultFileStore{dir: filepath.Join(dir, "secrets")}
}

func (s *defaultFileStore) Get(key string) ([]byte, error) {
	path, err := s.pathForKey(key)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("read secret: %w", err)
	}
	return data, nil
}

func (s *defaultFileStore) Put(key string, value []byte) error {
	path, err := s.pathForKey(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create secret dir: %w", err)
	}
	if err := os.WriteFile(path, value, 0o600); err != nil {
		return fmt.Errorf("write secret: %w", err)
	}
	return nil
}

func (s *defaultFileStore) Delete(key string) error {
	path, err := s.pathForKey(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return fmt.Errorf("delete secret: %w", err)
	}
	return nil
}

func (s *defaultFileStore) pathForKey(key string) (string, error) {
	if s.dir == "" {
		return "", fmt.Errorf("secret dir not available")
	}
	clean := defaultSanitizeKey(key)
	if clean == "" {
		return "", fmt.Errorf("invalid secret key")
	}
	return filepath.Join(s.dir, clean+".bin"), nil
}

func defaultSanitizeKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	key = strings.ReplaceAll(key, "/", "_")
	key = strings.ReplaceAll(key, "\\", "_")
	key = strings.ReplaceAll(key, "..", "_")
	return key
}
