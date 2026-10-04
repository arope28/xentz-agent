package cli

import (
	"errors"
	"strings"
	"testing"

	"xentz-agent/internal/state"
)

// lastRunWithStatus builds a minimal successful/failed LastRun for the
// status-problem classification tests.
func lastRunWithStatus(s string) state.LastRun {
	return state.LastRun{Status: s, TimeUTC: "2026-01-01T00:00:00Z"}
}

func TestParseFlagsHelpExitsZero(t *testing.T) {
	fs := newFlagSet("doctor")
	fs.String("mode", "user", "mode")

	for _, arg := range []string{"-h", "--help", "-help"} {
		help, err := parseFlags(fs, []string{arg})
		if err != nil {
			t.Errorf("parseFlags(%q) returned error %v, want nil", arg, err)
		}
		if !help {
			t.Errorf("parseFlags(%q) helpRequested = false, want true", arg)
		}
	}
}

func TestParseFlagsInvalidIsUsageError(t *testing.T) {
	fs := newFlagSet("doctor")
	fs.String("mode", "user", "mode")

	help, err := parseFlags(fs, []string{"--nope"})
	if help {
		t.Error("helpRequested = true for an invalid flag, want false")
	}
	if err == nil {
		t.Fatal("expected an error for an invalid flag")
	}
	// Invalid command lines must keep exiting 2, as they always have.
	code, alreadyReported := ExitCode(err)
	if code != 2 {
		t.Errorf("ExitCode = %d, want 2", code)
	}
	if !alreadyReported {
		t.Error("ExitCode reported the error as not-yet-reported; parseFlags already printed it")
	}
}

func TestParseFlagsPositionalArgsStillReachCommand(t *testing.T) {
	fs := newFlagSet("restore")
	fs.String("config", "", "config")

	help, err := parseFlags(fs, []string{"--config", "/tmp/c.json", "snapshots"})
	if err != nil || help {
		t.Fatalf("parseFlags err=%v help=%v", err, help)
	}
	if got := fs.Arg(0); got != "snapshots" {
		t.Errorf("Arg(0) = %q, want %q", got, "snapshots")
	}
}

func TestExitCodePlainErrorIsNotPreReported(t *testing.T) {
	// A normal error must still be printed once by main.
	if _, reported := ExitCode(errors.New("boom")); reported {
		t.Error("ExitCode marked a plain error as already reported")
	}
}

func TestRunHelpUnknownCommandStillSucceeds(t *testing.T) {
	// Asking about a typo should show help, not fail the command.
	if err := RunHelp([]string{"definitely-not-a-command"}); err != nil {
		t.Errorf("RunHelp(unknown) = %v, want nil", err)
	}
}

func TestRunHelpKnownCommand(t *testing.T) {
	if err := RunHelp([]string{"status"}); err != nil {
		t.Errorf("RunHelp(status) = %v, want nil", err)
	}
}

func TestCommandsMapCoversOrder(t *testing.T) {
	// Every listed command must have metadata, or it vanishes from `help`.
	for _, name := range commandOrder {
		if _, ok := commands[name]; !ok {
			t.Errorf("commandOrder lists %q but commands has no entry", name)
		}
	}
	for name := range commands {
		found := false
		for _, listed := range commandOrder {
			if listed == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("commands has %q but commandOrder omits it, so it never appears in help", name)
		}
	}
}

func TestGeneralUsageMentionsEveryCommand(t *testing.T) {
	usage := GeneralUsage()
	for _, name := range commandOrder {
		if !strings.Contains(usage, name) {
			t.Errorf("GeneralUsage does not mention %q", name)
		}
	}
}

func TestStatusProblems(t *testing.T) {
	yes := true
	no := false

	cases := []struct {
		name string
		rep  statusReport
		want string
	}{
		{"healthy", statusReport{HasBackup: true, Backup: lastRunWithStatus("success"), IncludeCount: 2}, ""},
		{"never ran", statusReport{IncludeCount: 1}, "no backup has run yet"},
		{"backup failed", statusReport{HasBackup: true, Backup: lastRunWithStatus("error"), IncludeCount: 1}, "the last backup failed"},
		{"kill switch", statusReport{HasBackup: true, Backup: lastRunWithStatus("success"), Enabled: &no, IncludeCount: 1}, "disabled by the server"},
		{"revoked", statusReport{HasBackup: true, Backup: lastRunWithStatus("success"), Revoked: true, Enabled: &yes, IncludeCount: 1}, "rejected this device"},
		{"no includes", statusReport{HasBackup: true, Backup: lastRunWithStatus("success")}, "no include paths"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			joined := strings.Join(statusProblems(tc.rep), " | ")
			if tc.want == "" {
				if joined != "" {
					t.Errorf("statusProblems = %q, want none", joined)
				}
				return
			}
			if !strings.Contains(joined, tc.want) {
				t.Errorf("statusProblems = %q, want it to mention %q", joined, tc.want)
			}
		})
	}
}

func TestFirstLine(t *testing.T) {
	cases := map[string]string{
		"":                        "(no error detail recorded)",
		"  simple  ":              "simple",
		"line one\nline two":      "line one ...",
		"only-one-line-with-nl\n": "only-one-line-with-nl", // trimmed before splitting, so no ellipsis
	}
	for in, want := range cases {
		if got := firstLine(in); got != want {
			t.Errorf("firstLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewFlagSetDiscardsOutput(t *testing.T) {
	// The usage dump must not reach stdout during a parse error; parseFlags
	// prints one concise line instead.
	fs := newFlagSet("doctor")
	fs.String("mode", "user", "mode")
	if got := fs.Output(); got == nil {
		t.Fatal("fs.Output() is nil")
	}
}

func TestFindingsDeduplicates(t *testing.T) {
	// Recording the same problem twice must not inflate the count.
	f := newFindings()
	f.record(severityProblem, "same thing")
	f.record(severityProblem, "same thing")
	if f.problems != 1 {
		t.Errorf("problems = %d, want 1 (duplicates should collapse)", f.problems)
	}
	f.record(severityWarning, "a warning")
	if f.warnings != 1 {
		t.Errorf("warnings = %d, want 1", f.warnings)
	}
	if f.healthy() {
		t.Error("healthy() = true with a recorded problem, want false")
	}
}
