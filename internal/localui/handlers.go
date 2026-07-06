package localui

import (
	"fmt"
	"net/http"
	"path/filepath"

	"xentz-agent/internal/config"
	"xentz-agent/internal/diagnostics"
	"xentz-agent/internal/paths"
	"xentz-agent/internal/report"
	"xentz-agent/internal/state"
)

func (s *Server) handleStatus(cfgPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		st, _ := state.New()
		lr, bOK, _ := st.LoadLastRun()
		lrt, rtOK, _ := st.LoadLastRetentionRun()
		agentState, _, _ := st.LoadAgentState()
		spoolCount, spoolBytes, _ := report.SpoolStats()

		cfg, cfgErr := config.Read(cfgPath)

		// API compat: ?json=1 returns raw JSON (only when we can read config)
		if r.URL.Query().Get("json") == "1" && cfgErr == nil {
			var lastBackupTime string
			if bOK {
				lastBackupTime = lr.TimeUTC
			}
			var retentionTime string
			if rtOK {
				retentionTime = lrt.TimeUTC
			}
			writeJSON(w, map[string]interface{}{
				"last_backup":     lastBackupTime,
				"last_retention":  retentionTime,
				"revoked":         agentState.Revoked,
				"spool_count":     spoolCount,
				"spool_bytes":     spoolBytes,
				"server_url":      cfg.ServerURL,
				"config_revision": cfg.ConfigRevision,
			})
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		var lastBackupTime, backupStatus string
		if bOK {
			lastBackupTime = lr.TimeUTC
			backupStatus = fmt.Sprintf("%s (%d files, %s)", lr.Status, lr.FilesTotal, func(b int64) string {
				unit := []string{"B", "KB", "MB", "GB", "TB"}
				if b <= 0 {
					return "0 B"
				}
				i := 0
				f := float64(b)
				for f >= 1024 && i < len(unit)-1 {
					f /= 1024
					i++
				}
				return fmt.Sprintf("%.1f %s", f, unit[i])
			}(lr.BytesSent))
		} else {
			lastBackupTime = "Never"
			backupStatus = "No backups recorded"
		}

		var retentionTime string
		if rtOK {
			retentionTime = lrt.TimeUTC
		} else {
			retentionTime = "Never"
		}

		data := StatusPageData{
			ServerURL:       "",
			ConfigRev:       "",
			ConfigFound:     cfgErr == nil,
			LastBackup:      backupStatus,
			LastBackupAt:    lastBackupTime,
			LastRetentionAt: retentionTime,
			Revoked:         agentState.Revoked,
			SpoolCount:      spoolCount,
			SpoolBytesHuman: humanizeBytes(int(spoolBytes)),
		}
		if cfgErr == nil {
			data.ConfigFound = true
			data.ServerURL = cfg.ServerURL
			data.ConfigRev = fmt.Sprintf("v%d", cfg.ConfigRevision)
		}
		statusTmpl.Execute(w, data)
	}
}

func (s *Server) handleConfig(cfgPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, cfgErr := config.Read(cfgPath)

		if cfgErr != nil && r.URL.Query().Get("json") == "1" {
			writeJSON(w, map[string]string{"error": cfgErr.Error()})
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		var enableBool bool = false
		if cfgErr == nil && cfg.Enabled != nil {
			enableBool = *cfg.Enabled
		}

		var scheduleStr string
		if cfgErr == nil {
			scheduleStr = fmt.Sprintf("Every 6h at midnight (revision %d)", cfg.ConfigRevision)
		} else {
			scheduleStr = "Using defaults"
		}

		data := ConfigPageData{
			ServerURL:    "",
			TenantID:     "",
			DeviceID:     "",
			UserID:       "",
			ConfigRev:    0,
			EnableState:  "Disabled",
			ScheduleCron: scheduleStr,
			IncludePaths: []string{},
			ExcludePaths: []string{},
			ResticRepo:   "",
			Retention:    "default",
			PasswordFile: "",
			ConfigFound:  false,
		}

		if cfgErr == nil {
			data.ServerURL = cfg.ServerURL
			data.TenantID = cfg.TenantID
			data.DeviceID = cfg.DeviceID
			data.UserID = cfg.UserID
			data.ConfigRev = cfg.ConfigRevision
			if enableBool {
				data.EnableState = "Enabled"
			} else {
				data.EnableState = "Disabled"
			}
			data.ScheduleCron = scheduleStr
			data.IncludePaths = cfg.Include
			data.ExcludePaths = cfg.Exclude
			data.ResticRepo = cfg.Restic.Repository
			data.Retention = "default"
			data.PasswordFile = cfg.Restic.PasswordFile
			data.ConfigFound = true
		}

		configTmpl.Execute(w, data)
	}
}

func (s *Server) handleRuns() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		st, _ := state.New()
		lastRun, _, _ := st.LoadLastRun()
		lastRetention, _, _ := st.LoadLastRetentionRun()
		writeJSON(w, map[string]interface{}{
			"backup":    lastRun,
			"retention": lastRetention,
		})
	}
}

func (s *Server) handleDiagnostics() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		stateDir, err := paths.StateDir("")
		if err != nil {
			writeErr(w, err)
			return
		}
		outPath := filepath.Join(stateDir, "diagnostics.zip")
		if err := diagnostics.CreateBundle(outPath); err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, map[string]string{"path": outPath})
	}
}
