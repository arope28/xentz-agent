package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type AgentState struct {
	Revoked   bool   `json:"revoked"`
	UpdatedAt string `json:"updated_at"`
}

func (s *Store) agentStatePath() string {
	return filepath.Join(s.dir, "agent_state.json")
}

func (s *Store) SaveAgentState(st AgentState) error {
	if st.UpdatedAt == "" {
		st.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.agentStatePath(), b, 0o600)
}

func (s *Store) LoadAgentState() (AgentState, bool, error) {
	b, err := os.ReadFile(s.agentStatePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return AgentState{}, false, nil
		}
		return AgentState{}, false, err
	}
	var st AgentState
	if err := json.Unmarshal(b, &st); err != nil {
		return AgentState{}, false, err
	}
	return st, true, nil
}

func (s *Store) SetRevoked(revoked bool) error {
	return s.SaveAgentState(AgentState{
		Revoked:   revoked,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	})
}
