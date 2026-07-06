package secretstore

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func setFakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XENTZ_MODE", "user")
	return home
}

func TestNewStoreFileOverride(t *testing.T) {
	setFakeHome(t)
	t.Setenv("XENTZ_AGENT_SECRETSTORE", "file")

	s := newStore()
	if _, ok := s.(*fileStore); !ok {
		t.Fatalf("expected *fileStore with XENTZ_AGENT_SECRETSTORE=file, got %T", s)
	}
}

func TestNewStoreDefaultIsPlatformStore(t *testing.T) {
	setFakeHome(t)
	t.Setenv("XENTZ_AGENT_SECRETSTORE", "")

	s := newStore()
	if runtime.GOOS == "darwin" {
		if _, ok := s.(*keychainStore); !ok {
			t.Fatalf("expected *keychainStore by default on darwin, got %T", s)
		}
	}
	if s == nil {
		t.Fatal("newStore returned nil")
	}
}

func TestFileStoreRoundTrip(t *testing.T) {
	home := setFakeHome(t)
	t.Setenv("XENTZ_AGENT_SECRETSTORE", "file")

	s := newStore()

	if _, err := s.Get("device_api_key_user"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound before Put, got %v", err)
	}

	if err := s.Put("device_api_key_user", []byte("secret-value")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := s.Get("device_api_key_user")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "secret-value" {
		t.Fatalf("Get = %q, want %q", got, "secret-value")
	}

	// Secrets must land under the fake HOME so per-user isolation holds.
	fs, ok := s.(*fileStore)
	if !ok {
		t.Fatalf("expected *fileStore, got %T", s)
	}
	if !strings.HasPrefix(fs.dir, home) {
		t.Fatalf("secret dir %q not under fake HOME %q", fs.dir, home)
	}

	path, err := fs.pathForKey("device_api_key_user")
	if err != nil {
		t.Fatalf("pathForKey: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat secret file: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("secret file mode = %v, want 0600", info.Mode().Perm())
	}

	if err := s.Delete("device_api_key_user"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get("device_api_key_user"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after Delete, got %v", err)
	}
	if err := s.Delete("device_api_key_user"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound deleting twice, got %v", err)
	}
}

func TestSanitizeKey(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"device_api_key", "device_api_key"},
		{"  padded  ", "padded"},
		{"", ""},
		{"   ", ""},
		{"a/b", "a_b"},
		{`a\b`, "a_b"},
		{"a..b", "a_b"},
		{"../../etc/passwd", "____etc_passwd"},
	}
	for _, tt := range tests {
		if got := sanitizeKey(tt.in); got != tt.want {
			t.Errorf("sanitizeKey(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
