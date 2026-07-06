//go:build darwin

package secretstore

import "testing"

func TestNewStoreDefaultIsKeychainOnDarwin(t *testing.T) {
	setFakeHome(t)
	t.Setenv("XENTZ_AGENT_SECRETSTORE", "")

	s := newStore()
	if _, ok := s.(*keychainStore); !ok {
		t.Fatalf("expected *keychainStore by default on darwin, got %T", s)
	}
}
