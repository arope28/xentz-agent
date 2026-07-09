package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fakePoster struct {
	paths []string
	fail  bool
}

func (f *fakePoster) PostJSON(path string, in, out any) error {
	f.paths = append(f.paths, path)
	if f.fail {
		return fmt.Errorf("post failed")
	}
	return nil
}

func newShipTestLogger(t *testing.T, poster jsonPoster) (*Logger, string) {
	t.Helper()
	dir := t.TempDir()
	writer, err := newRotatingWriter(filepath.Join(dir, "agent.log"))
	if err != nil {
		t.Fatalf("newRotatingWriter: %v", err)
	}
	logger := &Logger{
		writer: writer,
		newShipClient: func(serverURL, apiKey string) (jsonPoster, error) {
			return poster, nil
		},
	}
	t.Cleanup(func() { _ = logger.Close() })
	return logger, dir
}

func writeRotatedLog(t *testing.T, dir string, age time.Duration) string {
	t.Helper()
	path := filepath.Join(dir, "agent.log.2026-01-01T00-00-00.json")
	content := `{"timestamp":"2026-01-01T00:00:00Z","level":"info","message":"first"}
{"timestamp":"2026-01-01T00:00:01Z","level":"info","message":"second"}
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write rotated log: %v", err)
	}
	mtime := time.Now().Add(-age)
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	return path
}

// Log shipping must use the /control/ proxy prefix like every other agent
// endpoint: the public origin only proxies /control/* to the control plane and
// 404s bare /admin/... paths, so anything else silently strands device logs.
func TestShipLogsPostsRotatedFilesToControlProxyPath(t *testing.T) {
	poster := &fakePoster{}
	logger, dir := newShipTestLogger(t, poster)
	rotated := writeRotatedLog(t, dir, 2*time.Hour)

	if err := logger.ShipLogs("https://control.example.com", "device-key"); err != nil {
		t.Fatalf("ShipLogs: %v", err)
	}
	if len(poster.paths) != 1 {
		t.Fatalf("PostJSON calls = %d, want 1", len(poster.paths))
	}
	if poster.paths[0] != "/control/v1/logs" {
		t.Fatalf("path = %q, want /control/v1/logs", poster.paths[0])
	}
	if _, err := os.Stat(rotated); !os.IsNotExist(err) {
		t.Fatalf("rotated file should be deleted after shipping (stat err = %v)", err)
	}
}

func TestShipLogsSkipsFilesYoungerThanMinAge(t *testing.T) {
	poster := &fakePoster{}
	logger, dir := newShipTestLogger(t, poster)
	rotated := writeRotatedLog(t, dir, 10*time.Minute)

	if err := logger.ShipLogs("https://control.example.com", "device-key"); err != nil {
		t.Fatalf("ShipLogs: %v", err)
	}
	if len(poster.paths) != 0 {
		t.Fatalf("PostJSON calls = %d, want 0", len(poster.paths))
	}
	if _, err := os.Stat(rotated); err != nil {
		t.Fatalf("fresh rotated file should remain: %v", err)
	}
}

func TestShipLogsKeepsFileWhenShippingFails(t *testing.T) {
	poster := &fakePoster{fail: true}
	logger, dir := newShipTestLogger(t, poster)
	rotated := writeRotatedLog(t, dir, 2*time.Hour)

	// ShipLogs warns to stderr on per-file failures and returns nil.
	if err := logger.ShipLogs("https://control.example.com", "device-key"); err != nil {
		t.Fatalf("ShipLogs: %v", err)
	}
	if _, err := os.Stat(rotated); err != nil {
		t.Fatalf("rotated file must survive failed shipping: %v", err)
	}
}
