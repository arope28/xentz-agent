package backup

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"xentz-agent/internal/config"
	"xentz-agent/internal/state"
)

func Run(ctx context.Context, cfg config.Config) state.LastRun {
	start := time.Now()

	if len(cfg.Include) == 0 {
		return state.NewLastRunError(time.Since(start), 0, "no include paths configured")
	}
	if cfg.Restic.Repository == "" {
		return state.NewLastRunError(time.Since(start), 0, "restic.repository is required")
	}
	if cfg.Restic.PasswordFile == "" {
		return state.NewLastRunError(time.Since(start), 0, "restic.password_file is required (MVP)")
	}

	// Ensure restic exists
	if _, err := exec.LookPath("restic"); err != nil {
		return state.NewLastRunError(time.Since(start), 0, "restic not found in PATH (install restic first)")
	}

	// Optional: auto-init repo if needed (safe-ish for MVP; you can gate behind a flag later)
	if err := ensureRepoInitialized(ctx, cfg); err != nil {
		return state.NewLastRunError(time.Since(start), 0, "repo init check failed: "+err.Error())
	}

	args := []string{"backup", "--json"}
	for _, ex := range cfg.Exclude {
		args = append(args, "--exclude", ex)
	}
	// Consider adding: --one-file-system, --exclude-caches, etc. later.
	args = append(args, cfg.Include...)

	cmd := exec.CommandContext(ctx, "restic", args...)
	cmd.Env = append(cmd.Environ(),
		"RESTIC_REPOSITORY="+cfg.Restic.Repository,
		"RESTIC_PASSWORD_FILE="+expandHome(cfg.Restic.PasswordFile),
	)

	var out bytes.Buffer
	var jsonOut bytes.Buffer
	cmd.Stderr = &out     // Errors go to stderr
	cmd.Stdout = &jsonOut // JSON output goes to stdout

	err := cmd.Run()
	dur := time.Since(start)

	if err != nil {
		// Keep last ~8KB of output so status is readable
		msg := tail(out.String(), 8192)
		return state.NewLastRunError(dur, 0, "restic backup failed: "+err.Error()+"\n"+msg)
	}

	// Parse JSON output to extract stats
	stats := parseResticJSON(jsonOut.Bytes())
	if stats != nil {
		return state.NewLastRunSuccessWithStats(
			dur,
			stats.FilesTotal,
			stats.BytesTotal,
			stats.DataAddedBytes,
			stats.SnapshotID,
		)
	}

	// Fallback to basic success if JSON parsing fails
	return state.NewLastRunSuccess(dur, 0)
}

func ensureRepoInitialized(ctx context.Context, cfg config.Config) error {
	// "restic cat config" succeeds only if repo exists and is initialized
	cmd := exec.CommandContext(ctx, "restic", "cat", "config")
	cmd.Env = append(cmd.Environ(),
		"RESTIC_REPOSITORY="+cfg.Restic.Repository,
		"RESTIC_PASSWORD_FILE="+expandHome(cfg.Restic.PasswordFile),
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err == nil {
		return nil
	}

	// Try init (idempotency depends on rest-server path; if already initialized, init will error)
	initCmd := exec.CommandContext(ctx, "restic", "init")
	initCmd.Env = cmd.Env
	out.Reset()
	initCmd.Stdout = &out
	initCmd.Stderr = &out
	if err := initCmd.Run(); err != nil {
		// Include error output for debugging
		return fmt.Errorf("%v: %s", err, out.String())
	}
	return nil
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := filepath.Abs(strings.TrimSuffix(p, "/"))
		_ = home
	}
	// Minimal MVP: assume install writes absolute paths.
	return p
}

func tail(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[len(s)-max:]
}

// resticStats contains parsed statistics from restic JSON output
type resticStats struct {
	FilesTotal     int64
	BytesTotal     int64
	DataAddedBytes int64
	SnapshotID     string
}

// parseResticJSON parses restic JSON output and extracts summary statistics
func parseResticJSON(data []byte) *resticStats {
	// Restic outputs JSON objects, one per line
	scanner := bufio.NewScanner(bytes.NewReader(data))
	var summary map[string]interface{}

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var msg map[string]interface{}
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}

		// Look for summary message
		if msgType, ok := msg["message_type"].(string); ok && msgType == "summary" {
			summary = msg
			break
		}
	}

	if summary == nil {
		return nil
	}

	stats := &resticStats{}

	// Extract files_total (sum of files_new, files_changed, files_unmodified)
	if filesNew, ok := getFloat64(summary, "files_new"); ok {
		stats.FilesTotal += int64(filesNew)
	}
	if filesChanged, ok := getFloat64(summary, "files_changed"); ok {
		stats.FilesTotal += int64(filesChanged)
	}
	if filesUnmodified, ok := getFloat64(summary, "files_unmodified"); ok {
		stats.FilesTotal += int64(filesUnmodified)
	}

	// Extract bytes_total (total_bytes_processed)
	if bytesTotal, ok := getFloat64(summary, "total_bytes_processed"); ok {
		stats.BytesTotal = int64(bytesTotal)
	}

	// Extract data_added_bytes (bytes_added, not bytes_added_packed)
	if bytesAdded, ok := getFloat64(summary, "bytes_added"); ok {
		stats.DataAddedBytes = int64(bytesAdded)
	}

	// Extract snapshot_id
	if snapshotID, ok := summary["snapshot_id"].(string); ok {
		stats.SnapshotID = snapshotID
	}

	return stats
}

// getFloat64 safely extracts a float64 from a map, handling both float64 and int types
func getFloat64(m map[string]interface{}, key string) (float64, bool) {
	val, ok := m[key]
	if !ok {
		return 0, false
	}

	switch v := val.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	default:
		return 0, false
	}
}
