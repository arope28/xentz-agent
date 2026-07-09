package cli

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"xentz-agent/internal/backup"
	"xentz-agent/internal/config"
	"xentz-agent/internal/logging"
	"xentz-agent/internal/report"
	"xentz-agent/internal/state"
)

func RunBackup(args []string) error {
	fs := flag.NewFlagSet("backup", flag.ExitOnError)
	configPath := fs.String("config", "", "Config path override")
	autoInit := fs.Bool("auto-init", false, "Automatically initialize repository if it doesn't exist (use with caution)")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	cfgFile, err := config.ResolvePath(*configPath)
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}

	managed, err := loadManagedConfig("backup", cfgFile, true)
	if err != nil {
		return err
	}
	localCfg := managed.Local
	cfg := managed.Effective

	logger, err := logging.NewLogger(cfg.TenantID, cfg.DeviceID)
	if err != nil {
		log.Printf("warning: failed to initialize logger: %v", err)
		logger = nil
	} else {
		defer logger.Close()
		logger.SetComponent("backup")
	}

	st, err := state.New()
	if err != nil {
		if logger != nil {
			logger.Error("state init failed", err, nil)
		}
		return fmt.Errorf("state init: %w", err)
	}

	startTime := time.Now()

	if logger != nil {
		logger.Info("backup started", map[string]interface{}{
			"auto_init": *autoInit,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	defer cancel()

	res := backup.Run(ctx, cfg, *autoInit)
	if err := st.SaveLastRun(res); err != nil {
		if logger != nil {
			logger.Warn("failed to save last run", map[string]interface{}{
				"error": err.Error(),
			})
		}
		log.Printf("save last run: %v", err)
	}

	var logShipDone <-chan struct{}
	if logger != nil && localCfg.DeviceID != "" && localCfg.DeviceAPIKey != "" && localCfg.ServerURL != "" {
		logShipDone = shipLogsInBackground(logger, localCfg.ServerURL, localCfg.DeviceAPIKey)
	}

	if localCfg.DeviceID != "" && localCfg.DeviceAPIKey != "" && localCfg.ServerURL != "" {
		_ = report.SendPendingReports(localCfg.ServerURL, localCfg.DeviceAPIKey, 20)

		finishedTime := time.Now()
		reportStatus := "success"
		if res.Status == "error" {
			reportStatus = "failure"
		}
		backupReport := report.Report{
			DeviceID:       localCfg.DeviceID,
			ConfigRevision: cfg.ConfigRevision,
			Job:            "backup",
			StartedAt:      startTime.UTC().Format(time.RFC3339),
			FinishedAt:     finishedTime.UTC().Format(time.RFC3339),
			Status:         reportStatus,
			DurationMS:     res.DurationMS,
			FilesTotal:     res.FilesTotal,
			BytesTotal:     res.BytesTotal,
			DataAddedBytes: res.DataAddedBytes,
			SnapshotID:     res.SnapshotID,
		}
		if res.Error != "" {
			backupReport.Error = res.Error
		}

		_ = report.SendReportWithSpool(localCfg.ServerURL, localCfg.DeviceAPIKey, backupReport)
		_ = report.CleanupOldReports(30 * 24 * time.Hour)
	}

	if res.Status != "success" {
		if logger != nil {
			logger.Error("backup failed", fmt.Errorf("%s", res.Error), map[string]interface{}{
				"duration_ms": res.DurationMS,
			})
		}
		log.Printf("backup failed ❌: %s", res.Error)
		awaitLogShipping(logShipDone)
		os.Exit(1)
	}

	if logger != nil {
		logger.Info("backup completed successfully", map[string]interface{}{
			"duration_ms":      res.DurationMS,
			"bytes_sent":       res.BytesSent,
			"files_total":      res.FilesTotal,
			"bytes_total":      res.BytesTotal,
			"data_added_bytes": res.DataAddedBytes,
			"snapshot_id":      res.SnapshotID,
		})
	}
	log.Printf("backup ok ✅: duration=%s data_added=%s", res.Duration, formatBackupBytes(res.BytesSent))
	awaitLogShipping(logShipDone)
	return nil
}

func formatBackupBytes(n int64) string {
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
