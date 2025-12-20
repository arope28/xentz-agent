package install

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"xentz-agent/internal/config"
)

const (
	windowsTaskName = "xentz-agent"
)

func WindowsTaskSchedulerInstall(configPath string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("WindowsTaskSchedulerInstall can only run on Windows")
	}

	// Read config to get schedule time (HH:MM)
	cfg, err := config.Read(configPath)
	if err != nil {
		return err
	}
	hour, minute, err := parseHHMM(cfg.Schedule.DailyAt)
	if err != nil {
		return fmt.Errorf("invalid --daily-at (%q): %w", cfg.Schedule.DailyAt, err)
	}

	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	// Convert to Windows path format if needed
	exePath = filepath.Clean(exePath)
	if !filepath.IsAbs(exePath) {
		absPath, err := filepath.Abs(exePath)
		if err != nil {
			return fmt.Errorf("get absolute path: %w", err)
		}
		exePath = absPath
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	logDir := filepath.Join(home, ".xentz-agent", "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return err
	}
	stdoutPath := filepath.Join(logDir, "agent.out.log")
	stderrPath := filepath.Join(logDir, "agent.err.log")

	// Create a batch file wrapper to handle logging
	batchFile := filepath.Join(home, ".xentz-agent", "run-backup.bat")
	batchContent := fmt.Sprintf(`@echo off
"%s" backup --config "%s" >> "%s" 2>> "%s"
`, exePath, configPath, stdoutPath, stderrPath)
	
	if err := os.WriteFile(batchFile, []byte(batchContent), 0o644); err != nil {
		return fmt.Errorf("write batch file: %w", err)
	}

	// Delete existing task if it exists (ignore errors)
	_ = exec.Command("schtasks", "/Delete", "/TN", windowsTaskName, "/F").Run()

	// Create new scheduled task
	// Format: schtasks /Create /TN "TaskName" /TR "Command" /SC DAILY /ST HH:MM
	createCmd := exec.Command("schtasks", "/Create",
		"/TN", windowsTaskName,
		"/TR", fmt.Sprintf(`"%s"`, batchFile),
		"/SC", "DAILY",
		"/ST", fmt.Sprintf("%02d:%02d", hour, minute),
		"/F", // Force creation (overwrite if exists)
	)

	output, err := createCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("create scheduled task: %w\noutput: %s", err, string(output))
	}

	// Run the task immediately to test
	_ = exec.Command("schtasks", "/Run", "/TN", windowsTaskName).Run()

	return nil
}

