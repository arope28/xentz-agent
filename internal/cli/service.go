package cli

import (
	"flag"
	"fmt"
	"log"
	"os/exec"
	"runtime"

	"xentz-agent/internal/config"
	"xentz-agent/internal/install"
	windowsservice "xentz-agent/internal/service/windows"
)

func RunService(args []string) error {
	fs := flag.NewFlagSet("service", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if len(fs.Args()) < 1 {
		return fmt.Errorf("service requires subcommand: install|uninstall|start|stop")
	}
	if runtime.GOOS != "windows" {
		return fmt.Errorf("service command is only supported on Windows")
	}
	switch fs.Args()[0] {
	case "install":
		cfg := ""
		if len(fs.Args()) > 1 {
			cfg = fs.Args()[1]
		}
		if cfg == "" {
			cfgFile, err := config.ResolvePath("")
			if err != nil {
				return fmt.Errorf("resolve config path: %w", err)
			}
			cfg = cfgFile
		}
		if err := install.WindowsServiceInstall(cfg); err != nil {
			return fmt.Errorf("service install failed: %w", err)
		}
		log.Println("service install complete ✅")
		return nil
	case "uninstall":
		if err := install.WindowsServiceUninstall(); err != nil {
			return fmt.Errorf("service uninstall failed: %w", err)
		}
		log.Println("service uninstall complete ✅")
		return nil
	case "start":
		_ = exec.Command("sc", "start", "XentzAgent").Run()
		return nil
	case "stop":
		_ = exec.Command("sc", "stop", "XentzAgent").Run()
		return nil
	case "run":
		runFS := flag.NewFlagSet("service run", flag.ExitOnError)
		runConfig := runFS.String("config", "", "Config path for service run")
		if err := runFS.Parse(fs.Args()[1:]); err != nil {
			return fmt.Errorf("parse flags: %w", err)
		}
		if *runConfig == "" {
			cfgFile, err := config.ResolvePath("")
			if err != nil {
				return fmt.Errorf("resolve config path: %w", err)
			}
			*runConfig = cfgFile
		}
		if err := windowsservice.RunService(*runConfig); err != nil {
			return fmt.Errorf("service run failed: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported service subcommand")
	}
}
