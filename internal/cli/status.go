package cli

import (
	"flag"
	"fmt"

	"xentz-agent/internal/config"
	"xentz-agent/internal/report"
	"xentz-agent/internal/state"
)

func RunStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	configPath := fs.String("config", "", "Config path override")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	st, err := state.New()
	if err != nil {
		return fmt.Errorf("state init: %w", err)
	}

	configRevision := 0
	cfgFile, err := config.ResolvePath(*configPath)
	if err != nil {
		fmt.Printf("warning: resolve config path: %v\n", err)
	}
	if cfgFile != "" {
		if cfg, err := config.Read(cfgFile); err == nil {
			if mmErr := ModeMismatchError("status", cfg); mmErr != nil {
				fmt.Printf("Mode warning: %v\n", mmErr)
			}
			configRevision = cfg.ConfigRevision
		}
	}
	if configRevision == 0 {
		if cachedCfg, err := config.ReadCached(); err == nil {
			configRevision = cachedCfg.ConfigRevision
		}
	}

	spoolCount, spoolBytes, _ := report.SpoolStats()

	revoked := false
	if agentState, ok, _ := st.LoadAgentState(); ok {
		revoked = agentState.Revoked
	}

	last, ok, err := st.LoadLastRun()
	if err != nil {
		return fmt.Errorf("load last run: %w", err)
	}
	if !ok {
		fmt.Println("No backups have run yet.")
	} else {
		fmt.Printf("Last backup:\n  status: %s\n  time:   %s\n  dur:    %s\n  data_added: %s\n  error:  %s\n",
			last.Status, last.TimeUTC, last.Duration, formatStatusBytes(last.BytesSent), last.Error)
	}

	lastRetention, ok, err := st.LoadLastRetentionRun()
	if err != nil {
		return fmt.Errorf("load last retention run: %w", err)
	}
	if ok {
		fmt.Println("")
		fmt.Printf("Last retention:\n  status: %s\n  time:   %s\n  dur:    %s\n  error:  %s\n",
			lastRetention.Status, lastRetention.TimeUTC, lastRetention.Duration, lastRetention.Error)
	}

	fmt.Println("")
	fmt.Printf("Config revision: %d\n", configRevision)
	fmt.Printf("Spool backlog: %d report(s), %d bytes\n", spoolCount, spoolBytes)
	fmt.Printf("Revoked: %v\n", revoked)
	return nil
}

func formatStatusBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB"}
	s := float64(n) / float64(div)
	if s == float64(int64(s)) {
		return fmt.Sprintf("%d %s", int64(s), units[exp])
	}
	return fmt.Sprintf("%.1f %s", s, units[exp])
}
