package shutdown

import (
	"context"
	"time"
)

// startWatchdog starts a goroutine that hard-exits the process if the
// budget plus grace elapses while shutdown is still running. Returns a
// stop function that cancels the watchdog goroutine.
//
// The watchdog only fires if budget > 0; with no budget the process is
// expected to coordinate its own deadline elsewhere.
func (m *Manager) startWatchdog(_ context.Context, start time.Time) func() {
	if m.cfg.budget <= 0 {
		return func() {}
	}
	stop := make(chan struct{})

	go func() {
		// Wait for budget+grace from start, OR until stopped.
		deadline := start.Add(m.cfg.budget + m.cfg.watchdogGrace)
		t := time.NewTimer(time.Until(deadline))
		defer t.Stop()

		select {
		case <-stop:
			return
		case <-t.C:
			m.cfg.logger.Error("shutdown: watchdog deadline exceeded — forcing exit",
				"budget", m.cfg.budget,
				"grace", m.cfg.watchdogGrace,
				"failureExitCode", m.cfg.failureExitCode,
			)
			m.exitFn(m.cfg.failureExitCode)
		}
	}()

	return func() { close(stop) }
}
