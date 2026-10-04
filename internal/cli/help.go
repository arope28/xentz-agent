package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// command describes a subcommand for the help screens. Flag details are NOT
// duplicated here: `xentz-agent <command> --help` prints the authoritative
// list straight from that command's own FlagSet, so the two can never drift.
type command struct {
	summary  string
	usage    string // positional-argument shape, e.g. "guided | snapshots | <snapshot_id>"
	examples []string
}

var commands = map[string]command{
	"version": {
		summary: "Print agent version/build information",
	},
	"install": {
		summary: "Enroll the device and install the scheduled task",
		examples: []string{
			"xentz-agent install --token <install-token> --server <control-plane-url> --include \"/Users/me/Documents\"",
			"xentz-agent install --repo rest:https://... --include \"/Users/me/Documents\"   # legacy mode",
		},
	},
	"doctor": {
		summary: "Diagnose enrollment, secrets, restic and server connectivity",
		examples: []string{
			"xentz-agent doctor --check-server",
		},
	},
	"recover": {
		summary: "Recover enrollment after config loss, using a portal recovery token",
		examples: []string{
			"xentz-agent recover --server <control-plane-url> --recovery-token <token>",
		},
	},
	"uninstall": {
		summary: "Remove the scheduled task, and optionally purge state and credentials",
		examples: []string{
			"xentz-agent uninstall --mode user",
			"xentz-agent uninstall --mode user --purge-state",
		},
	},
	"upgrade": {
		summary: "Replace the binary and restart the scheduler or service",
		examples: []string{
			"xentz-agent upgrade --binary /path/to/new/xentz-agent --mode system",
		},
	},
	"diagnostics": {
		summary: "Create a support bundle (logs + config checksum + state)",
		examples: []string{
			"xentz-agent diagnostics --out /tmp/xentz-agent-diag.zip",
		},
	},
	"local-ui": {
		summary: "Run the localhost-only status and restore UI",
		examples: []string{
			"xentz-agent local-ui --addr 127.0.0.1:9800",
		},
	},
	"backup": {
		summary: "Run one backup now (this is what the scheduler invokes)",
		examples: []string{
			"xentz-agent backup",
			"xentz-agent backup --auto-init   # create the repository if it does not exist",
		},
	},
	"restore": {
		summary: "Browse and restore snapshots (restic wrapper)",
		usage:   "guided | snapshots | find <path> | ls <snapshot_id> [path] | stats [snapshot_id] | check | <snapshot_id> --target <dir> [--path <path>] | dump <snapshot_id> <path> [--output <file>]",
		examples: []string{
			"xentz-agent restore guided          # step-by-step, easiest for most people",
			"xentz-agent restore snapshots",
			"xentz-agent restore find /path/to/file",
			"xentz-agent restore ls latest",
			"xentz-agent restore check",
			"xentz-agent restore <snapshot_id> --target /tmp/restore",
		},
	},
	"retention": {
		summary: "Run the retention/prune policy",
	},
	"status": {
		summary: "Show whether backups are healthy, and when they last ran",
		examples: []string{
			"xentz-agent status",
			"xentz-agent status --json        # for monitoring scripts",
		},
	},
	"config": {
		summary: "Manage backup paths (add/remove include and exclude patterns)",
		examples: []string{
			`xentz-agent config --add-include "/Users/me/Documents"`,
			`xentz-agent config --add-exclude "*.tmp"`,
			"xentz-agent config --list-all",
		},
	},
}

// commandOrder fixes the order of the command list in the help output.
var commandOrder = []string{
	"version", "install", "doctor", "recover", "uninstall", "upgrade",
	"diagnostics", "local-ui", "backup", "restore", "retention", "status", "config",
}

// RunHelp implements `xentz-agent help [command]`. It always succeeds: asking
// for help is not an error.
func RunHelp(args []string) error {
	if len(args) == 0 {
		fmt.Print(GeneralUsage())
		return nil
	}

	name := args[0]
	cmd, ok := commands[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", name)
		fmt.Print(GeneralUsage())
		return nil
	}

	fmt.Printf("%s - %s\n\n", name, cmd.summary)
	fmt.Printf("Usage:\n  xentz-agent %s", name)
	if cmd.usage != "" {
		fmt.Printf(" %s", cmd.usage)
	}
	fmt.Println(" [flags]")

	if len(cmd.examples) > 0 {
		fmt.Println("\nExamples:")
		for _, ex := range cmd.examples {
			fmt.Printf("  %s\n", ex)
		}
	}

	fmt.Printf("\nFlags:\n  xentz-agent %s --help\n", name)
	return nil
}

// printCommandUsage is the FlagSet.Usage hook: a summary followed by the real
// flag definitions, so `--help` can never fall out of date with the code.
func printCommandUsage(name string, fs *flag.FlagSet) {
	cmd, ok := commands[name]
	if ok {
		fmt.Printf("%s - %s\n\n", name, cmd.summary)
		fmt.Printf("Usage:\n  xentz-agent %s", name)
		if cmd.usage != "" {
			fmt.Printf(" %s", cmd.usage)
		}
		fmt.Println(" [flags]")
		fmt.Println()
	} else {
		fmt.Printf("Usage:\n  xentz-agent %s [flags]\n\n", name)
	}

	fmt.Println("Flags:")
	// The FlagSet's output is discarded so parse errors are not double-printed;
	// send PrintDefaults back to stdout for the help path.
	fs.SetOutput(os.Stdout)
	fs.PrintDefaults()
	fs.SetOutput(io.Discard)
}

// GeneralUsage is the top-level help text.
func GeneralUsage() string {
	var b strings.Builder
	b.WriteString(`xentz-agent - cross-platform backup agent (restic)

Usage:
  xentz-agent <command> [flags]

Commands:
`)
	for _, name := range commandOrder {
		cmd := commands[name]
		b.WriteString(fmt.Sprintf("  %-12s %s\n", name, cmd.summary))
	}

	b.WriteString(`
Getting started:
  1. Install restic (the backup engine): `)
	if hint := resticInstallHint(); hint != "" {
		b.WriteString(hint)
	} else {
		b.WriteString("https://restic.net")
	}
	b.WriteString(`
  2. Enroll this device:
       xentz-agent install --token <install-token> --server <control-plane-url> --include "/Users/me/Documents"
  3. Verify:
       xentz-agent status     # is it healthy? (quick glance)
       xentz-agent doctor     # if not, why not? (detailed check)

Common tasks:
  xentz-agent backup                  run a backup now
  xentz-agent status                  health at a glance
  xentz-agent status --json           machine-readable, for monitoring
  xentz-agent doctor --check-server   deep check including the server
  xentz-agent restore guided          step-by-step restore
  xentz-agent config --add-include "/Users/me/Pictures"
  xentz-agent local-ui                status and restore dashboard

More help:
  xentz-agent help <command>          examples for one command
  xentz-agent <command> --help        every flag of one command

Notes:
  With token-based enrollment the control plane supplies the configuration and
  retention policy on each run; 'config' then updates the server. In legacy mode
  the same command edits config.json locally.
`)
	return b.String()
}
