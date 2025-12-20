package install

import (
	"fmt"
	"runtime"
)

// Install installs the agent scheduler for the current operating system
func Install(configPath string) error {
	switch runtime.GOOS {
	case "darwin":
		return MacOSLaunchdInstall(configPath)
	case "windows":
		return WindowsTaskSchedulerInstall(configPath)
	case "linux":
		return LinuxSystemdInstall(configPath)
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

