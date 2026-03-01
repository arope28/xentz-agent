package install

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"xentz-agent/internal/config"
	"xentz-agent/internal/paths"
	windowsservice "xentz-agent/internal/service/windows"
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

	logDir, err := paths.LogDir("")
	if err != nil {
		return fmt.Errorf("resolve log dir: %w", err)
	}
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return err
	}
	stdoutPath := filepath.Join(logDir, "agent.out.log")
	stderrPath := filepath.Join(logDir, "agent.err.log")

	goBin := filepath.Join(home, "go", "bin")
	localBin := filepath.Join(home, "AppData", "Local", "Programs")
	pathEnv := fmt.Sprintf("%s;%s;%%PATH%%", goBin, localBin)

	stateDir, err := paths.StateDir("")
	if err != nil {
		return fmt.Errorf("resolve state dir: %w", err)
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return err
	}
	batchFile := filepath.Join(stateDir, "run-backup.bat")
	batchContent := fmt.Sprintf(`@echo off
set PATH=%s
"%s" backup --auto-init --config "%s" >> "%s" 2>> "%s"
`, pathEnv, exePath, configPath, stdoutPath, stderrPath)

	if err := os.WriteFile(batchFile, []byte(batchContent), 0o644); err != nil {
		return fmt.Errorf("write batch file: %w", err)
	}

	_ = exec.Command("schtasks", "/Delete", "/TN", windowsTaskName, "/F").Run()
	createCmd := exec.Command("schtasks", "/Create",
		"/TN", windowsTaskName,
		"/TR", fmt.Sprintf(`"%s"`, batchFile),
		"/SC", "DAILY",
		"/ST", fmt.Sprintf("%02d:%02d", hour, minute),
		"/F",
	)
	if output, err := createCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create scheduled task: %w\noutput: %s", err, string(output))
	}
	_ = exec.Command("schtasks", "/Run", "/TN", windowsTaskName).Run()
	return nil
}

func WindowsTaskSchedulerUninstall() error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("WindowsTaskSchedulerUninstall can only run on Windows")
	}
	_ = exec.Command("schtasks", "/Delete", "/TN", windowsTaskName, "/F").Run()
	return nil
}

// WindowsServiceInstall installs a Windows Service for system mode.
func WindowsServiceInstall(configPath string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("WindowsServiceInstall can only run on Windows")
	}
	return windowsservice.InstallService(configPath)
}

func WindowsServiceUninstall() error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("WindowsServiceUninstall can only run on Windows")
	}
	return windowsservice.UninstallService()
}
