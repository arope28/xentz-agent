package cli

import (
	"flag"
	"fmt"
	"log"
	"os"

	"xentz-agent/internal/config"
	"xentz-agent/internal/identity"
	"xentz-agent/internal/install"
	"xentz-agent/internal/paths"
	"xentz-agent/internal/secretstore"
)

func RunUninstall(args []string) error {
	fs := flag.NewFlagSet("uninstall", flag.ExitOnError)
	mode := fs.String("mode", "user", "Uninstall mode: user or system")
	keepConfig := fs.Bool("keep-config", true, "Keep config directory")
	purgeState := fs.Bool("purge-state", false, "Remove state/log directories")
	configPath := fs.String("config", "", "Config path override")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	cfgFile, err := resolveConfigPathWithMode(*configPath, *mode)
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}

	if err := install.UninstallWithMode(cfgFile, *mode); err != nil {
		return fmt.Errorf("uninstall failed: %w", err)
	}

	if !*keepConfig {
		cfgDir, err := paths.ConfigDir(*mode)
		if err == nil {
			_ = os.RemoveAll(cfgDir)
		}
	}

	if *purgeState {
		stateDir, err := paths.StateDir(*mode)
		if err == nil {
			_ = os.RemoveAll(stateDir)
		}
		logDir, err := paths.LogDir(*mode)
		if err == nil {
			_ = os.RemoveAll(logDir)
		}
		_ = identity.Delete(*mode)
		_ = config.DeleteDeviceAPIKeysForMode(*mode)
		_ = secretstore.Delete(secretstore.KeyResticPassword)
	}

	log.Println("uninstall complete ✅")
	return nil
}
