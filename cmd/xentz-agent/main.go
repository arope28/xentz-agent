package main

import (
	"fmt"
	"log"
	"os"

	"xentz-agent/internal/cli"
	"xentz-agent/internal/version"
)

func usage() {
	fmt.Print(`xentz-agent - Backup Agent

Commands:
  version     Print agent version/build information
  install     Install config + scheduled task (macOS: launchd, Windows: Task Scheduler/Service, Linux: systemd/cron)
  doctor      Print enrollment/secret diagnostics and optional server auth check
  recover     Recover enrollment after config loss (portal recovery token)
  uninstall   Remove service/scheduler and optionally purge config/state
  upgrade     Replace binary and restart service/scheduler
  diagnostics Create a support bundle (logs + config checksum + state)
  local-ui    Run localhost-only status UI
  backup      Run one backup now (used by scheduler)
  restore     List/browse snapshots, find files, stats, check repo, or restore (restic wrapper)
  retention   Run retention/prune policy (forget old snapshots)
  status      Show last run status
  config      Manage backup paths (add/remove include/exclude paths)

Examples:
  # Token-based enrollment (recommended):
  xentz-agent install --token <install-token> --server https://control-plane.example.com --daily-at 02:00 --include "/Users/me/Documents"
  
  # Recover after a reinstall/restore (requires portal-minted token):
  xentz-agent recover --server https://control-plane.example.com --recovery-token <token>
  
  # Legacy mode (direct repository):
  xentz-agent install --repo rest:https://... --password "..." --daily-at 02:00 --include "/Users/me/Documents"
  
  xentz-agent backup
  xentz-agent backup --auto-init  # Auto-initialize repository if missing (use with caution)
  xentz-agent restore snapshots
  xentz-agent restore guided
  xentz-agent restore find /path/to/file
  xentz-agent restore ls latest
  xentz-agent restore stats
  xentz-agent restore check
  xentz-agent restore <snapshot_id> --target /tmp/restore [--path /path/to/file]
  xentz-agent restore dump <snapshot_id> /path/in/snapshot --output ./file
  xentz-agent retention
  xentz-agent status
  
  # Manage backup paths:
  xentz-agent config --add-include "/Users/me/Documents" --add-include "/Users/me/Pictures"
  xentz-agent config --remove-include "/Users/me/Pictures"
  xentz-agent config --add-exclude "*.tmp"
  xentz-agent config --list-all
  
  # Uninstall (keep config by default):
  xentz-agent uninstall --mode user
  
  # Upgrade:
  xentz-agent upgrade --binary /path/to/new/xentz-agent --mode system
  
  # Diagnostics:
  xentz-agent diagnostics --out /tmp/xentz-agent-diag.zip
  
  # Local UI:
  xentz-agent local-ui --addr 127.0.0.1:9800

Flags (backup):
  --config       Config path override (default: <CONFIG_DIR>/config.json)
  --auto-init    Automatically initialize repository if it doesn't exist (default: false)
                 WARNING: Only use if you're certain the repository URL is correct.
                 Without this flag, backup will fail if repository doesn't exist.

Flags (restore):
  --config       Config path override (default: <CONFIG_DIR>/config.json)
  Subcommands:    guided | snapshots | find <path> | ls <snapshot_id> [path] | stats [snapshot_id] | check |
                  <snapshot_id> --target <dir> [--path <path>] | dump <snapshot_id> <path> [--output <file>]

Flags (retention):
  --config       Config path override (default: <CONFIG_DIR>/config.json)

Flags (status):
  --config       Config path override (default: <CONFIG_DIR>/config.json)

Flags (install):
  --token         Install token for enrollment (recommended, provided by control plane)
  --server        Control plane base URL (required with --token)
  --daily-at      Time in HH:MM (24h), default 02:00
  --mode          Install mode: user or system (default: user)
  --force         Replace existing enrollment (clears stored API key + local identity before enroll)
  --repo          Restic repository URL (legacy mode, use --token instead)
  --password      Restic repository password (optional if server provides via enrollment)
  --password-file Path to restic password file (optional, default: <CONFIG_DIR>/restic.pw)
  --include       Repeatable. Add include paths. Example: --include "/Users/me/Documents" --include "/Users/me/Pictures"
  --exclude       Repeatable. Add exclude globs.
  --config        Config path override (default: <CONFIG_DIR>/config.json)

Flags (uninstall):
  --mode         Uninstall mode: user or system (default: user)
  --keep-config  Keep config directory (default: true)
  --purge-state  Remove state/log directories (default: false)
  --config       Config path override (default: <CONFIG_DIR>/config.json)

Flags (upgrade):
  --binary  Path to new xentz-agent binary (required)
  --mode    Upgrade mode: user or system (default: user)
  --config  Config path override (default: <CONFIG_DIR>/config.json)

Flags (diagnostics):
  --out     Output path for diagnostics bundle

Flags (local-ui):
  --addr    Bind address (default: 127.0.0.1:9800)
  --config  Config path override (default: <CONFIG_DIR>/config.json)

Flags (config):
  --add-include <path>      Add include path (repeatable). Paths are normalized (expanded and made absolute)
  --remove-include <path>   Remove include path (repeatable)
  --add-exclude <pattern>    Add exclude pattern (repeatable)
  --remove-exclude <pattern> Remove exclude pattern (repeatable)
  --list-includes            List current include paths
  --list-excludes            List current exclude patterns
  --list-all                 List both include paths and exclude patterns
  --config                   Config path override (default: <CONFIG_DIR>/config.json)

Flags (doctor):
  --mode          Inspect mode: user or system (default: user)
  --config        Config path override (default: <CONFIG_DIR>/config.json)
  --check-server  Attempt GET /control/v1/config and print HTTP status

Note: With token-based enrollment, configuration (including retention policy) is fetched from the server on each run.
      In legacy mode, retention policy must be configured in config.json before running 'retention' command.
      The 'config' command updates server configuration for enrolled devices, or local config for legacy mode.
`)
}

// run dispatches a command and returns the process exit code.
//
// A command that already reported its failure to the user returns a
// cli exitError; we honour its code without printing the message again.
func run(run func([]string) error, args []string) int {
	if err := run(args); err != nil {
		if code, ok := cli.ExitCode(err); ok {
			return code
		}
		log.Fatal(err)
	}
	return 0
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "-version") {
		fmt.Println(version.String())
		return
	}

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]

	switch cmd {
	case "version":
		fmt.Println(version.String())
		return

	case "install":
		os.Exit(run(cli.RunInstall, os.Args[2:]))

	case "doctor":
		os.Exit(run(cli.RunDoctor, os.Args[2:]))

	case "recover":
		os.Exit(run(cli.RunRecover, os.Args[2:]))

	case "uninstall":
		os.Exit(run(cli.RunUninstall, os.Args[2:]))

	case "upgrade":
		os.Exit(run(cli.RunUpgrade, os.Args[2:]))

	case "diagnostics":
		os.Exit(run(cli.RunDiagnostics, os.Args[2:]))

	case "local-ui":
		os.Exit(run(cli.RunLocalUI, os.Args[2:]))

	case "service":
		os.Exit(run(cli.RunService, os.Args[2:]))

	case "backup":
		os.Exit(run(cli.RunBackup, os.Args[2:]))

	case "restore":
		os.Exit(run(cli.RunRestore, os.Args[2:]))

	case "retention":
		os.Exit(run(cli.RunRetention, os.Args[2:]))

	case "status":
		os.Exit(run(cli.RunStatus, os.Args[2:]))

	case "config":
		os.Exit(run(cli.RunConfig, os.Args[2:]))

	default:
		usage()
		os.Exit(2)
	}
}
