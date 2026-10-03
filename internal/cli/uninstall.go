package cli

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	"xentz-agent/internal/config"
	"xentz-agent/internal/identity"
	"xentz-agent/internal/install"
	"xentz-agent/internal/paths"
	"xentz-agent/internal/secretstore"
)

func RunUninstall(args []string) error {
	fs := newFlagSet("uninstall")
	mode := fs.String("mode", "user", "Uninstall mode: user or system")
	keepConfig := fs.Bool("keep-config", true, "Keep config directory")
	purgeState := fs.Bool("purge-state", false, "Permanently delete state, logs and stored credentials")
	yes := fs.Bool("yes", false, "Skip the confirmation prompt")
	configPath := fs.String("config", "", "Config path override")
	if help, err := parseFlags(fs, args); err != nil {
		return err
	} else if help {
		return nil
	}

	cfgFile, err := resolveConfigPathWithMode(*configPath, *mode)
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}

	// Confirm before removing anything, not just before the destructive part:
	// declining should leave a working agent behind.
	if *purgeState || !*keepConfig {
		if !*yes && !confirmDestructive(*mode, *purgeState, !*keepConfig, cfgFile) {
			fmt.Println("Cancelled. Nothing was removed.")
			return nil
		}
	}

	if err := install.UninstallWithMode(cfgFile, *mode); err != nil {
		return fmt.Errorf("uninstall failed: %w", err)
	}

	if !*keepConfig {
		cfgDir, err := paths.ConfigDir(*mode)
		if err == nil {
			_ = os.RemoveAll(cfgDir)
			fmt.Printf("Removed %s\n", cfgDir)
		}
	}

	if *purgeState {
		stateDir, err := paths.StateDir(*mode)
		if err == nil {
			_ = os.RemoveAll(stateDir)
			fmt.Printf("Removed %s\n", stateDir)
		}
		logDir, err := paths.LogDir(*mode)
		if err == nil {
			_ = os.RemoveAll(logDir)
			fmt.Printf("Removed %s\n", logDir)
		}
		_ = identity.Delete(*mode)
		_ = config.DeleteDeviceAPIKeysForMode(*mode)
		_ = secretstore.Delete(secretstore.KeyResticPassword)
		fmt.Println("Deleted the stored device API key, enrollment identity, and restic password.")
		fmt.Println("To back up this machine again you must enroll with a new token:")
		fmt.Println("  xentz-agent recover --server <control-plane-url> --recovery-token <token>")
	}

	log.Println("uninstall complete ✅")
	if !*purgeState && *keepConfig {
		if _, statErr := os.Stat(cfgFile); statErr == nil {
			fmt.Println("Your configuration was kept. Run `xentz-agent install` again to resume backing up.")
		}
	}
	return nil
}

// confirmDestructive explains exactly what will be permanently deleted and
// asks for confirmation. When stdin is not a terminal (CI, provisioning
// scripts) it proceeds without prompting, so automation is never left hanging.
func confirmDestructive(mode string, purgeState, dropConfig bool, cfgFile string) bool {
	fmt.Println()
	fmt.Println("This will permanently delete:")
	if dropConfig {
		if dir, err := paths.ConfigDir(mode); err == nil {
			fmt.Printf("  - %s (configuration and enrollment)\n", dir)
		}
	} else {
		fmt.Printf("  - %s (configuration kept)\n", cfgFile)
	}
	if purgeState {
		if dir, err := paths.StateDir(mode); err == nil {
			fmt.Printf("  - %s (run history, queued reports)\n", dir)
		}
		if dir, err := paths.LogDir(mode); err == nil {
			fmt.Printf("  - %s (agent logs)\n", dir)
		}
		fmt.Println("  - the stored device API key and enrollment identity")
		fmt.Println("  - the stored restic repository password")
		fmt.Println("Existing snapshots in your backup repository are NOT affected.")
	}
	fmt.Println()

	if !stdinIsTerminal() {
		fmt.Println("Not an interactive terminal; continuing without confirmation.")
		return true
	}

	fmt.Print("Type 'yes' to continue: ")
	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil && answer == "" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(answer), "yes")
}

// stdinIsTerminal reports whether stdin is attached to a character device.
// Used to avoid hanging unattended installs/uninstalls waiting for input.
func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
