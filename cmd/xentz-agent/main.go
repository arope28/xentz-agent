package main

import (
	"fmt"
	"log"
	"os"

	"xentz-agent/internal/cli"
	"xentz-agent/internal/version"
)

func usage() {
	fmt.Print(cli.GeneralUsage())
}

// run dispatches a command and returns the process exit code.
//
// Errors that a command already reported to the user (a failed backup, an
// invalid command line) come back carrying their exit code; main must not
// print those again. Anything else is printed once, here, without a log
// timestamp so terminal output stays readable.
func run(run func([]string) error, args []string) int {
	if err := run(args); err != nil {
		if code, alreadyReported := cli.ExitCode(err); alreadyReported {
			return code
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

func main() {
	// Progress and errors go to the terminal, where a date/microsecond prefix
	// on every line is noise. Timestamps live in the structured JSON log file
	// instead (see internal/logging).
	log.SetFlags(0)

	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "-version") {
		fmt.Println(version.String())
		return
	}

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]

	// Asking for help is a successful request, not a usage error.
	if cmd == "help" || cmd == "-h" || cmd == "--help" {
		os.Exit(run(cli.RunHelp, os.Args[2:]))
	}

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
