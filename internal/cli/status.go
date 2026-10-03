package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"xentz-agent/internal/config"
	"xentz-agent/internal/report"
	"xentz-agent/internal/state"
)

// statusReport is the data behind `xentz-agent status`, shared by the
// human-readable rendering and the --json output so the two can never drift.
type statusReport struct {
	HasBackup      bool          `json:"has_backup"`
	Backup         state.LastRun `json:"last_backup"`
	HasRetention   bool          `json:"has_retention"`
	Retention      state.LastRun `json:"last_retention"`
	ConfigRev      int           `json:"config_revision"`
	DailyAt        string        `json:"daily_at"`
	IncludeCount   int           `json:"include_count"`
	Enabled        *bool         `json:"enabled"` // nil: server has not set a kill-switch
	Revoked        bool          `json:"revoked"` // device API key was rejected
	SpoolCount     int           `json:"spool_count"`
	SpoolBytes     int64         `json:"spool_bytes"`
	ModeWarning    string        `json:"mode_warning,omitempty"`
	RepoConfigured bool          `json:"repo_configured"`
}

func RunStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	configPath := fs.String("config", "", "Config path override")
	asJSON := fs.Bool("json", false, "Print machine-readable JSON")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	rep, err := collectStatus(*configPath)
	if err != nil {
		return err
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}

	printStatusReport(rep)
	return nil
}

func collectStatus(configPath string) (statusReport, error) {
	rep := statusReport{}

	st, err := state.New()
	if err != nil {
		return rep, fmt.Errorf("state init: %w", err)
	}

	cfgFile, err := config.ResolvePath(configPath)
	if err != nil {
		fmt.Printf("warning: resolve config path: %v\n", err)
	}
	if cfgFile != "" {
		if cfg, err := config.Read(cfgFile); err == nil {
			if mmErr := ModeMismatchError("status", cfg); mmErr != nil {
				rep.ModeWarning = mmErr.Error()
			}
			rep.ConfigRev = cfg.ConfigRevision
			rep.DailyAt = cfg.Schedule.DailyAt
			rep.IncludeCount = len(cfg.Include)
			rep.Enabled = cfg.Enabled
			rep.RepoConfigured = cfg.Restic.Repository != ""
		}
	}
	if rep.ConfigRev == 0 {
		if cachedCfg, err := config.ReadCached(); err == nil {
			rep.ConfigRev = cachedCfg.ConfigRevision
		}
	}

	if agentState, ok, _ := st.LoadAgentState(); ok {
		rep.Revoked = agentState.Revoked
	}

	last, ok, err := st.LoadLastRun()
	if err != nil {
		return rep, fmt.Errorf("load last run: %w", err)
	}
	rep.HasBackup, rep.Backup = ok, last

	lastRetention, ok, err := st.LoadLastRetentionRun()
	if err != nil {
		return rep, fmt.Errorf("load last retention run: %w", err)
	}
	rep.HasRetention, rep.Retention = ok, lastRetention

	rep.SpoolCount, rep.SpoolBytes, _ = report.SpoolStats()
	return rep, nil
}

func printStatusReport(rep statusReport) {
	fmt.Println("Backup agent status")
	fmt.Println()

	if rep.ModeWarning != "" {
		fmt.Printf("⚠  %s\n", rep.ModeWarning)
	}

	// Device state first: if the server disabled this device, that explains
	// every stale number below it.
	switch {
	case rep.Revoked:
		fmt.Println("Device          API KEY REJECTED by the server")
		fmt.Println("                Backups cannot run until the device is re-enrolled.")
	case rep.Enabled != nil && !*rep.Enabled:
		fmt.Println("Device          DISABLED by server (kill switch)")
		fmt.Println("                Backups and restores are stopped until the server re-enables it.")
	case rep.Enabled != nil:
		fmt.Println("Device          enabled")
	default:
		fmt.Println("Device          no kill-switch set by server")
	}
	fmt.Println()

	printRun("Last backup", rep.HasBackup, rep.Backup)
	printRun("Last retention", rep.HasRetention, rep.Retention)

	if rep.SpoolCount == 0 {
		fmt.Println("Pending reports none (everything has been delivered)")
	} else {
		fmt.Printf("Pending reports %d report(s) waiting to send (%s)\n", rep.SpoolCount, formatBackupBytes(rep.SpoolBytes))
	}

	fmt.Printf("Config          revision %d", rep.ConfigRev)
	if strings.TrimSpace(rep.DailyAt) != "" {
		fmt.Printf(" · %s", describeSchedulePhrase(rep.DailyAt))
	}
	if rep.IncludeCount > 0 {
		fmt.Printf(" · %d path(s) included", rep.IncludeCount)
	} else {
		fmt.Printf(" · no include paths configured")
	}
	fmt.Println()

	if !rep.RepoConfigured {
		fmt.Println("⚠  No repository configured - run `xentz-agent install` or `xentz-agent recover`.")
	}
}

// printRun renders one run in the two-line form: a headline with the age of the
// run, then the details that are actually populated.
func printRun(label string, ok bool, run state.LastRun) {
	if !ok {
		fmt.Printf("%-16s never run\n\n", label+":")
		return
	}

	headline := fmt.Sprintf("%-16s %s · %s", label+":", run.Status, describeAge(run.TimeUTC))
	if local := formatLocalTime(run.TimeUTC); local != "" {
		headline += " (" + local + ")"
	}
	fmt.Println(headline)

	var details []string
	if run.DataAddedBytes > 0 {
		details = append(details, formatBackupBytes(run.DataAddedBytes)+" added")
	} else if run.BytesSent > 0 {
		details = append(details, formatBackupBytes(run.BytesSent)+" sent")
	}
	if run.FilesTotal > 0 {
		details = append(details, fmt.Sprintf("%d files scanned", run.FilesTotal))
	}
	if run.Duration != "" {
		details = append(details, "took "+run.Duration)
	}
	if run.SnapshotID != "" {
		details = append(details, "snapshot "+run.SnapshotID)
	}
	if len(details) > 0 {
		fmt.Printf("%-16s %s\n", "", strings.Join(details, " · "))
	}
	// Only surface an error when there is one.
	if strings.TrimSpace(run.Error) != "" {
		fmt.Printf("%-16s %s\n", "", strings.TrimSpace(run.Error))
	}
	fmt.Println()
}

// formatLocalTime renders an RFC3339 UTC timestamp in the user's own timezone.
// Returns "" if the value cannot be parsed, so callers can omit it.
func formatLocalTime(utc string) string {
	t, err := time.Parse(time.RFC3339, utc)
	if err != nil {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04") + " local"
}

// describeAge renders how long ago something happened, in words a user reads
// quickly ("2 hours ago" rather than a raw UTC timestamp).
func describeAge(utc string) string {
	t, err := time.Parse(time.RFC3339, utc)
	if err != nil {
		return "at " + utc
	}
	d := time.Since(t)
	if d < 0 {
		return "in the future (check the system clock)"
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return pluralize(int(d.Minutes()), "minute") + " ago"
	case d < 24*time.Hour:
		return pluralize(int(d.Hours()), "hour") + " ago"
	default:
		return pluralize(int(d.Hours()/24), "day") + " ago"
	}
}

func pluralize(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
