package backup

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"time"

	"xentz-agent/internal/config"
	"xentz-agent/internal/state"
)

func RunRetention(ctx context.Context, cfg config.Config) state.LastRun {
	start := time.Now()

	if cfg.Restic.Repository == "" {
		return state.NewLastRunError(time.Since(start), 0, "restic.repository is required")
	}
	if cfg.Restic.PasswordFile == "" {
		return state.NewLastRunError(time.Since(start), 0, "restic.password_file is required")
	}
	if _, err := exec.LookPath("restic"); err != nil {
		return state.NewLastRunError(time.Since(start), 0, "restic not found in PATH")
	}

	// Check repository connectivity with a short timeout before proceeding
	// This prevents hanging if the repository server is down
	os.Stderr.WriteString("Checking repository connectivity...\n")
	connectCtx, connectCancel := context.WithTimeout(ctx, 30*time.Second)
	defer connectCancel()
	if err := checkRepositoryConnectivity(connectCtx, cfg); err != nil {
		if connectCtx.Err() == context.DeadlineExceeded {
			return state.NewLastRunError(time.Since(start), 0, "repository connection timeout: repository server appears to be unreachable or down\nCheck that the repository server is online and accessible.")
		}
		return state.NewLastRunError(time.Since(start), 0, "repository not reachable: "+err.Error()+"\nCheck that the repository server is online and accessible.")
	}
	os.Stderr.WriteString("Repository is reachable. Starting retention/prune operation...\n")

	args := []string{"forget"}

	r := cfg.Retention
	// If user never set retention, refuse to run (prevents accidental nukes / weird defaults)
	if r.KeepLast == 0 && r.KeepDaily == 0 && r.KeepWeekly == 0 && r.KeepMonthly == 0 && r.KeepYearly == 0 {
		return state.NewLastRunError(time.Since(start), 0, "retention policy not configured (set keep_* values)")
	}

	if r.KeepLast > 0 {
		args = append(args, "--keep-last", itoa(r.KeepLast))
	}
	if r.KeepDaily > 0 {
		args = append(args, "--keep-daily", itoa(r.KeepDaily))
	}
	if r.KeepWeekly > 0 {
		args = append(args, "--keep-weekly", itoa(r.KeepWeekly))
	}
	if r.KeepMonthly > 0 {
		args = append(args, "--keep-monthly", itoa(r.KeepMonthly))
	}
	if r.KeepYearly > 0 {
		args = append(args, "--keep-yearly", itoa(r.KeepYearly))
	}

	if r.Prune {
		args = append(args, "--prune")
	}

	cmd := exec.CommandContext(ctx, "restic", args...)
	cmd.Env = append(cmd.Environ(),
		"RESTIC_REPOSITORY="+cfg.Restic.Repository,
		"RESTIC_PASSWORD_FILE="+expandHome(cfg.Restic.PasswordFile),
	)

	// Stream output to both terminal and buffer for error reporting
	// This allows users to see progress during long-running prune operations
	var out bytes.Buffer
	tee := &teeWriter{buf: &out, stream: true}
	cmd.Stdout = tee
	cmd.Stderr = tee

	err := cmd.Run()
	dur := time.Since(start)

	if err != nil {
		return state.NewLastRunError(dur, 0, "restic forget/prune failed: "+err.Error()+"\n"+tail(out.String(), 8192))
	}
	return state.NewLastRunSuccess(dur, 0)
}

// tiny helpers (avoid fmt import in hot path)
func itoa(i int) string {
	// minimal
	buf := []byte{}
	n := i
	if n == 0 {
		return "0"
	}
	for n > 0 {
		buf = append([]byte{byte('0' + (n % 10))}, buf...)
		n /= 10
	}
	return string(buf)
}

// expandHome and tail are defined in backup.go (same package)

// checkRepositoryConnectivity verifies the repository is reachable with a quick test
func checkRepositoryConnectivity(ctx context.Context, cfg config.Config) error {
	// Use a quick "snapshots" command with --last 1 to test connectivity
	// This is faster than "cat config" and will fail quickly if unreachable
	cmd := exec.CommandContext(ctx, "restic", "snapshots", "--last", "1")
	cmd.Env = append(cmd.Environ(),
		"RESTIC_REPOSITORY="+cfg.Restic.Repository,
		"RESTIC_PASSWORD_FILE="+expandHome(cfg.Restic.PasswordFile),
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	// This will fail quickly if the repository is unreachable
	if err := cmd.Run(); err != nil {
		// Check if it's a context timeout (repository unreachable)
		if ctx.Err() == context.DeadlineExceeded {
			return context.DeadlineExceeded
		}
		// For other errors (like no snapshots), that's okay - at least we connected
		// Only return error if it looks like a connectivity issue
		errStr := out.String()
		if contains(errStr, "dial") || contains(errStr, "connection") || contains(errStr, "timeout") || contains(errStr, "refused") {
			return err
		}
		// If it's just "no snapshots found" or similar, that's fine - repo is reachable
	}
	return nil
}

func contains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// teeWriter writes to both a buffer and stdout for streaming output
type teeWriter struct {
	buf    *bytes.Buffer
	stream bool
}

func (t *teeWriter) Write(p []byte) (n int, err error) {
	// Write to buffer
	n, err = t.buf.Write(p)
	if err != nil {
		return n, err
	}
	// Also write to stdout for real-time progress
	if t.stream {
		os.Stdout.Write(p)
	}
	return n, nil
}