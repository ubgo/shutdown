package shutdown

import (
	"context"
	"sync"
	"time"
)

// runPhase runs all registrations in a phase. When serial==true, handlers
// run sequentially in name order. Otherwise (default), they run in parallel.
//
// Returns the slice of errors observed (empty if every handler succeeded
// or non-required handlers' errors were filtered out).
func (m *Manager) runPhase(parent context.Context, phase Phase, regs []registration, serial bool) []error {
	if len(regs) == 0 {
		return nil
	}

	results := make([]error, len(regs))

	if serial {
		for i, r := range regs {
			results[i] = m.runOneInPhase(parent, phase, r)
			if parent.Err() != nil {
				// Budget exhausted mid-phase. Mark remaining as not-run.
				for j := i + 1; j < len(regs); j++ {
					m.cfg.logger.Warn("shutdown: handler skipped, budget exhausted",
						"name", regs[j].name, "phase", phase.String())
				}
				break
			}
		}
	} else {
		var wg sync.WaitGroup
		for i, r := range regs {
			wg.Add(1)
			go func(i int, r registration) {
				defer wg.Done()
				results[i] = m.runOneInPhase(parent, phase, r)
			}(i, r)
		}
		wg.Wait()
	}

	out := []error{}
	for _, err := range results {
		if err == nil {
			continue
		}
		out = append(out, err)
	}
	return out
}

// runOneInPhase wraps runHandler with observer fan-out and timing.
func (m *Manager) runOneInPhase(parent context.Context, phase Phase, r registration) error {
	m.fireOnHandlerStart(r.name, phase)
	start := time.Now()
	err := m.runHandler(parent, r)
	dur := time.Since(start)
	m.fireOnHandlerEnd(r.name, phase, dur, err)

	if err != nil {
		m.cfg.logger.Error("shutdown: handler failed",
			"name", r.name, "phase", phase.String(), "duration", dur, "err", err)
	} else {
		m.cfg.logger.Info("shutdown: handler completed",
			"name", r.name, "phase", phase.String(), "duration", dur)
	}
	return err
}
