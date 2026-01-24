package identity

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"xentz-agent/internal/paths"
)

// Identity is the durable enrollment record, stored separately from policy config.
// This allows the agent to recover even if config.json is deleted.
type Identity struct {
	ServerURL   string `json:"server_url,omitempty"`
	TenantID    string `json:"tenant_id,omitempty"`
	DeviceID    string `json:"device_id,omitempty"`
	PrincipalID string `json:"principal_id,omitempty"`
	Mode        string `json:"mode,omitempty"`
}

func pathFor(mode string) (string, error) {
	stateDir, err := paths.StateDir(mode)
	if err != nil {
		return "", err
	}
	return filepath.Join(stateDir, "identity.json"), nil
}

func Load(mode string) (Identity, error) {
	p, err := pathFor(mode)
	if err != nil {
		return Identity{}, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return Identity{}, err
	}
	var id Identity
	if err := json.Unmarshal(b, &id); err != nil {
		return Identity{}, err
	}
	return id, nil
}

func Save(mode string, id Identity) error {
	p, err := pathFor(mode)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(id, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o600)
}

func Delete(mode string) error {
	p, err := pathFor(mode)
	if err != nil {
		return err
	}
	_ = os.Remove(p)
	return nil
}

// GetOrCreatePrincipalID returns a stable principal ID (UUID-like).
// We use a random 16-byte hex string to avoid adding dependencies.
func GetOrCreatePrincipalID(mode string) (string, error) {
	id, err := Load(mode)
	if err == nil && strings.TrimSpace(id.PrincipalID) != "" {
		return strings.TrimSpace(id.PrincipalID), nil
	}

	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	// RFC4122-ish v4 markers (still hex string output).
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	principalID := hex.EncodeToString(b[:])

	id.PrincipalID = principalID
	id.Mode = mode
	if err := Save(mode, id); err != nil {
		return "", err
	}
	return principalID, nil
}
