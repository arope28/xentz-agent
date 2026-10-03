package cli

import (
	"context"
	"fmt"
	"log"
	"time"

	"xentz-agent/internal/backup"
	"xentz-agent/internal/config"
	"xentz-agent/internal/logging"
	"xentz-agent/internal/report"
	"xentz-agent/internal/state"
)

func RunRetention(args []string) error {
	fs := newFlagSet("retention")
	configPath := fs.String("config", "", "Config path override")
	if help, err := parseFlags(fs, args); err != nil {
		return err
	} else if help {
		return nil
	}

	cfgFile, err := config.ResolvePath(*configPath)
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}

	managed, err := loadManagedConfig("retention", cfgFile, true)
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
		logger.SetComponent("retention")
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
		logger.Info("retention started", nil)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	res := backup.RunRetention(ctx, cfg)
	if err := st.SaveLastRetentionRun(res); err != nil {
		if logger != nil {
			logger.Warn("failed to save last retention run", map[string]interface{}{
				"error": err.Error(),
			})
		}
		log.Printf("save last retention run: %v", err)
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
		retentionReport := report.Report{
			DeviceID:       localCfg.DeviceID,
			ConfigRevision: cfg.ConfigRevision,
			Job:            "retention",
			StartedAt:      startTime.UTC().Format(time.RFC3339),
			FinishedAt:     finishedTime.UTC().Format(time.RFC3339),
			Status:         reportStatus,
			DurationMS:     res.DurationMS,
		}
		if res.Error != "" {
			retentionReport.Error = res.Error
		}

		_ = report.SendReportWithSpool(localCfg.ServerURL, localCfg.DeviceAPIKey, retentionReport)
		_ = report.CleanupOldReports(30 * 24 * time.Hour)
	}

	if res.Status != "success" {
		if logger != nil {
			logger.Error("retention failed", fmt.Errorf("%s", res.Error), map[string]interface{}{
				"duration_ms": res.DurationMS,
			})
		}
		log.Printf("retention failed ❌: %s", res.Error)
		awaitLogShipping(logShipDone)
		// Return rather than os.Exit so the deferred logger.Close still runs.
		return failWithCode(fmt.Errorf("retention failed: %s", res.Error), 1)
	}

	if logger != nil {
		logger.Info("retention completed successfully", map[string]interface{}{
			"duration_ms": res.DurationMS,
		})
	}
	log.Printf("retention ok ✅: duration=%s", res.Duration)
	awaitLogShipping(logShipDone)
	return nil
}
