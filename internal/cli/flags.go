package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

// newFlagSet builds a FlagSet for a subcommand.
//
// Two deliberate differences from flag.ExitOnError:
//   - output goes to stdout, because help is requested, not an error report;
//   - -h/--help surfaces as flag.ErrHelp so the command can exit 0, whereas an
//     invalid flag still exits 2.
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard) // we render errors and usage ourselves
	fs.Usage = func() { printCommandUsage(name, fs) }
	return fs
}

// parseFlags parses args for a FlagSet built by newFlagSet.
//
// The flag package prints its own message and usage dump on any parse error.
// That is too noisy for an unknown flag, so usage is suppressed during Parse
// and this function decides what the user sees:
//   - -h/--help prints the full help and reports success (exit 0);
//   - an invalid flag prints one concise line plus a pointer to --help, and
//     exits 2 as it always has.
func parseFlags(fs *flag.FlagSet, args []string) (helpRequested bool, err error) {
	realUsage := fs.Usage
	fs.Usage = func() {} // discard the flag package's usage dump
	parseErr := fs.Parse(args)
	fs.Usage = realUsage

	if parseErr != nil {
		if errors.Is(parseErr, flag.ErrHelp) {
			realUsage()
			return true, nil
		}
		fmt.Fprintf(os.Stderr, "xentz-agent %s: %v\n", fs.Name(), parseErr)
		fmt.Fprintf(os.Stderr, "Run 'xentz-agent %s --help' for usage.\n", fs.Name())
		return false, &usageError{err: parseErr}
	}
	return false, nil
}

// usageError marks an invalid command line. main exits 2 without reprinting,
// because parseFlags already explained the problem.
type usageError struct{ err error }

func (e *usageError) Error() string { return e.err.Error() }

func (e *usageError) Unwrap() error { return e.err }
