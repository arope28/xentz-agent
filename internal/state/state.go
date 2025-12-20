package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

type LastRun struct {
	Status    string `json:"status"` // success|error
	TimeUTC   string `json:"time_utc"`
	Duration  string `json:"duration"`
	BytesSent int64  `json:"bytes_sent"`
	Error     string `json:"error,omitempty"`
}

type Store struct {
	dir string
}

func New() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".xentz-agent")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

func (s *Store) lastRunPath() string {
	return filepath.Join(s.dir, "last_run.json")
}

func (s *Store) SaveLastRun(r LastRun) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.lastRunPath(), b, 0o600)
}

func (s *Store) LoadLastRun() (LastRun, bool, error) {
	b, err := os.ReadFile(s.lastRunPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return LastRun{}, false, nil
		}
		return LastRun{}, false, err
	}
	var r LastRun
	if err := json.Unmarshal(b, &r); err != nil {
		return LastRun{}, false, err
	}
	return r, true, nil
}

func NewLastRunSuccess(d time.Duration, bytes int64) LastRun {
	return LastRun{
		Status:    "success",
		TimeUTC:   time.Now().UTC().Format(time.RFC3339),
		Duration:  d.String(),
		BytesSent: bytes,
	}
}

func NewLastRunError(d time.Duration, bytes int64, msg string) LastRun {
	return LastRun{
		Status:    "error",
		TimeUTC:   time.Now().UTC().Format(time.RFC3339),
		Duration:  d.String(),
		BytesSent: bytes,
		Error:     msg,
	}
}

func (s *Store) lastRetentionPath() string {
	return filepath.Join(s.dir, "last_retention.json")
}

func (s *Store) SaveLastRetentionRun(r LastRun) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.lastRetentionPath(), b, 0o600)
}

func (s *Store) LoadLastRetentionRun() (LastRun, bool, error) {
	b, err := os.ReadFile(s.lastRetentionPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return LastRun{}, false, nil
		}
		return LastRun{}, false, err
	}
	var r LastRun
	if err := json.Unmarshal(b, &r); err != nil {
		return LastRun{}, false, err
	}
	return r, true, nil
}