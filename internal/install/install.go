package install

import (
	"fmt"
	"runtime"
	"strings"
)

// Install installs the agent scheduler for the current operating system (default: user mode)
func Install(configPath string) error {
	return InstallWithMode(configPath, "")
}

// InstallWithMode installs scheduler/service for the requested mode (user/system)
func InstallWithMode(configPath, mode string) error {
	m := strings.ToLower(strings.TrimSpace(mode))
	if m == "" {
		m = "user"
	}
	switch runtime.GOOS {
	case "darwin":
		if m == "system" {
			return MacOSLaunchdInstallSystem(configPath)
		}
		return MacOSLaunchdInstall(configPath)
	case "windows":
		if m == "system" {
			return WindowsServiceInstall(configPath)
		}
		return WindowsTaskSchedulerInstall(configPath)
	case "linux":
		if m == "system" {
			return LinuxSystemdInstallSystem(configPath)
		}
		return LinuxSystemdInstall(configPath)
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// UninstallWithMode removes scheduler/service for the requested mode (user/system)
func UninstallWithMode(configPath, mode string) error {
	m := strings.ToLower(strings.TrimSpace(mode))
	if m == "" {
		m = "user"
	}
	switch runtime.GOOS {
	case "darwin":
		if m == "system" {
			return MacOSLaunchdUninstallSystem(configPath)
		}
		return MacOSLaunchdUninstall(configPath)
	case "windows":
		if m == "system" {
			return WindowsServiceUninstall()
		}
		return WindowsTaskSchedulerUninstall()
	case "linux":
		if m == "system" {
			return LinuxSystemdUninstallSystem()
		}
		return LinuxSystemdUninstall()
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}
