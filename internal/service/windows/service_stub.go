//go:build !windows

package windows

import "fmt"

const ServiceName = "XentzAgent"

func RunService(configPath string) error {
	return fmt.Errorf("windows service not supported on this platform")
}

func InstallService(configPath string) error {
	return fmt.Errorf("windows service not supported on this platform")
}

func UninstallService() error {
	return fmt.Errorf("windows service not supported on this platform")
}
