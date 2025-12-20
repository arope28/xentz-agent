package backup

import (
	"bytes"
	"context"
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

	args := []string{"backup"}
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
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	dur := time.Since(start)

	if err != nil {
		// Keep last ~8KB of output so status is readable
		msg := tail(out.String(), 8192)
		return state.NewLastRunError(dur, 0, "restic backup failed: "+err.Error()+"\n"+msg)
	}

	return state.NewLastRunSuccess(dur, 0) // bytes_sent can be parsed later (or from restic --json)
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
		return err
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