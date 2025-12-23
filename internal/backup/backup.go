package backup

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"xentz-agent/internal/config"
	"xentz-agent/internal/state"
)

func Run(ctx context.Context, cfg config.Config, autoInit bool) state.LastRun {
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

	// Check if repository exists and is initialized
	// Only auto-init if explicitly enabled (prevents accidental repo creation)
	if err := checkOrInitRepo(ctx, cfg, autoInit); err != nil {
		return state.NewLastRunError(time.Since(start), 0, "repo init check failed: "+err.Error())
	}

	args := []string{"backup", "--json"}
	for _, ex := range cfg.Exclude {
		args = append(args, "--exclude", ex)
	}
	// Consider adding: --one-file-system, --exclude-caches, etc. later.
	// Add -- before include paths to prevent flag injection if paths start with -
	args = append(args, "--")
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

// checkOrInitRepo checks if the repository exists and is initialized.
// If autoInit is true and the repo doesn't exist, it will attempt to initialize it.
// If autoInit is false and the repo doesn't exist, it returns an error.
func checkOrInitRepo(ctx context.Context, cfg config.Config, autoInit bool) error {
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
		// Repository exists and is initialized
		return nil
	}

	// Repository doesn't exist or isn't initialized
	if !autoInit {
		return fmt.Errorf("repository does not exist or is not initialized (use --auto-init to automatically initialize, or run 'restic init' manually)")
	}

	// Auto-init is enabled, attempt to initialize
	// Note: This is idempotent - if already initialized, init will return an error
	// but we'll catch that and return a clearer message
	initCmd := exec.CommandContext(ctx, "restic", "init")
	initCmd.Env = cmd.Env
	out.Reset()
	initCmd.Stdout = &out
	initCmd.Stderr = &out
	if err := initCmd.Run(); err != nil {
		// Check if error is because repo already exists (idempotency)
		errStr := out.String()
		if strings.Contains(errStr, "already initialized") || strings.Contains(errStr, "config file already exists") {
			// Repository was initialized between check and init (race condition) or already exists
			return nil
		}
		return fmt.Errorf("failed to initialize repository: %w\noutput: %s", err, errStr)
	}
	return nil
}

func expandHome(p string) string {
	// Handle ~ or ~/... paths
	if p == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return p // Return original if we can't get home dir
		}
		return home
	}
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p // Return original if we can't get home dir
		}
		// Replace ~ with home directory and join the rest
		expanded := filepath.Join(home, p[2:])
		// Normalize to absolute path
		abs, err := filepath.Abs(expanded)
		if err != nil {
			return expanded // Return expanded path even if Abs fails
		}
		return abs
	}
	// If path doesn't start with ~, normalize to absolute path if relative
	if !filepath.IsAbs(p) {
		abs, err := filepath.Abs(p)
		if err != nil {
			return p // Return original if Abs fails
		}
		return abs
	}
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
