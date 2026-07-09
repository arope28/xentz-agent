package cli

import (
	"log"
	"time"

	"xentz-agent/internal/logging"
)

// logShipExitWait bounds how long a run waits for background log shipping before
// exiting, so an unreachable control plane cannot stall backup/retention exit.
const logShipExitWait = 60 * time.Second

// shipLogsInBackground ships rotated logs to the control plane without blocking
// the run. Callers must pass the returned channel to awaitLogShipping before the
// process exits; otherwise the process can die between a successful ship and the
// rotated file's deletion, re-shipping duplicate entries on the next run.
func shipLogsInBackground(logger *logging.Logger, serverURL, apiKey string) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := logger.ShipLogs(serverURL, apiKey); err != nil {
			log.Printf("warning: failed to ship logs: %v", err)
		}
	}()
	return done
}

// awaitLogShipping waits for shipLogsInBackground to finish, bounded by
// logShipExitWait. A nil channel (shipping never started) returns immediately.
func awaitLogShipping(done <-chan struct{}) {
	if done == nil {
		return
	}
	select {
	case <-done:
	case <-time.After(logShipExitWait):
		log.Printf("warning: log shipping still running at exit; rotated logs remain for the next run")
	}
}
