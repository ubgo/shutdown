package shutdown

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"
)

// Manager is the central shutdown coordinator.
//
// Construct with New, register handlers (and optionally actors), then call
// Listen (blocking on signals) or Shutdown (programmatic). A Manager is
// safe for concurrent Register and Subscribe calls before Listen/Shutdown
// has been entered; once a shutdown is in progress further Register calls
// return ErrClosed.
type Manager struct {
	cfg config

	mu        sync.RWMutex
	handlers  map[string]registration
	actors    []*actorRegistration
	observers []Observer
	closed    bool

	// shutdownOnce guards the actual phase execution so a programmatic
	// Shutdown call followed by a signal (or vice versa) does not run
	// phases twice.
	shutdownOnce sync.Once

	// signalHooks maps custom signals (SIGHUP etc.) to user callbacks.
	// These fire instead of triggering a shutdown.
	signalHooksMu sync.RWMutex
	signalHooks   map[os.Signal]func(ctx context.Context, sig os.Signal)
}

// ErrClosed is returned by Register when the Manager has already started
// running its phases.
var ErrClosed = errors.New("shutdown: manager closed (shutdown in progress or completed)")

// ErrAlreadyRegistered is returned by Register when a handler with the
// given name is already in the Manager.
var ErrAlreadyRegistered = errors.New("shutdown: handler already registered with that name")

// ErrEmptyName is returned by Register when name is the empty string.
var ErrEmptyName = errors.New("shutdown: handler name must be non-empty")

// New constructs a Manager with the supplied options.
func New(opts ...Option) *Manager {
	m := &Manager{
		handlers:    make(map[string]registration),
		signalHooks: make(map[os.Signal]func(ctx context.Context, sig os.Signal)),
	}
	m.cfg = defaultConfig()
	for _, o := range opts {
		o(&m.cfg)
	}
	if m.cfg.logger == nil {
		m.cfg.logger = defaultLogger()
	}
	return m
}

// Subscribe attaches an Observer. Multiple observers can coexist; each one
// receives every callback in the order they were subscribed.
func (m *Manager) Subscribe(o Observer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observers = append(m.observers, o)
}

// OnSignal registers a hook for a non-shutdown signal (e.g. SIGHUP for
// config reload, SIGUSR1 for log rotation). When the signal arrives the
// hook is invoked with a fresh context derived from the Listen ctx; the
// shutdown sequence is NOT triggered.
//
// Pass shutdown.ContinueListening to stay in the signal loop after the
// hook returns; the manager keeps listening for further signals (including
// SIGTERM/SIGINT) until either Listen's ctx cancels or a shutdown signal
// arrives. The default behaviour is ContinueListening.
//
// Note: signals registered here are no longer treated as shutdown triggers
// by the manager. If a user adds SIGTERM via OnSignal it will not start a
// shutdown — it will only call the user's hook.
func (m *Manager) OnSignal(sig os.Signal, fn func(ctx context.Context, sig os.Signal)) {
	m.signalHooksMu.Lock()
	defer m.signalHooksMu.Unlock()
	m.signalHooks[sig] = fn
}

// Listen blocks until one of the configured shutdown signals arrives or
// ctx is cancelled, then runs all phases in order. Returns the aggregated
// error (errors.Join of every handler error) or context.Canceled if the
// caller cancelled before completion.
//
// Listen does NOT call os.Exit by default; opt in via WithExitOnComplete.
//
// While shutdown is running a second shutdown signal triggers an immediate
// os.Exit(forceCode) when WithForceOnSecondSignal is enabled (default).
//
// Listen is safe to call once per Manager; subsequent calls return ErrClosed.
func (m *Manager) Listen(ctx context.Context) error {
	signals := append([]os.Signal{}, m.cfg.signals...)
	if len(signals) == 0 {
		signals = []os.Signal{syscall.SIGINT, syscall.SIGTERM}
	}

	sigCh := make(chan os.Signal, 4)
	signal.Notify(sigCh, signals...)
	defer signal.Stop(sigCh)

	// Loop until a shutdown signal arrives or ctx cancels.
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case sig := <-sigCh:
			// Is this a non-shutdown hook signal?
			m.signalHooksMu.RLock()
			hook, hooked := m.signalHooks[sig]
			m.signalHooksMu.RUnlock()
			if hooked {
				// User-defined hook wins. Registering OnSignal for SIGTERM
				// means "react to SIGTERM but do NOT shut down" — the hook
				// is the user's escape hatch.
				m.cfg.logger.Info("shutdown: signal hook fired", "signal", sig.String())
				if hook != nil {
					hook(ctx, sig)
				}
				continue
			}

			// It's a shutdown signal. Begin shutdown, but keep the channel
			// alive so we can detect a second signal for force-exit.
			m.fireOnSignal(sig)
			runErr := m.runShutdownWithForceWatch(ctx, sigCh)
			return runErr
		}
	}
}

// Shutdown is the programmatic equivalent of receiving a signal. Runs the
// same phase machinery as Listen.
func (m *Manager) Shutdown(ctx context.Context) error {
	return m.runShutdown(ctx)
}

// runShutdownWithForceWatch starts shutdown in a goroutine and watches the
// signal channel for a second signal (force exit) while shutdown is in
// progress.
func (m *Manager) runShutdownWithForceWatch(ctx context.Context, sigCh chan os.Signal) error {
	type result struct{ err error }
	done := make(chan result, 1)
	go func() {
		done <- result{err: m.runShutdown(ctx)}
	}()

	for {
		select {
		case sig := <-sigCh:
			if m.cfg.forceOnSecondSignal {
				m.cfg.logger.Warn("shutdown: second signal received — forcing exit",
					"signal", sig.String(), "exitCode", m.cfg.forceExitCode)
				m.exitFn(m.cfg.forceExitCode)
				return nil // unreachable when exitFn is os.Exit; reachable in tests with injected exit fn
			}
			m.cfg.logger.Info("shutdown: second signal received but force-exit disabled",
				"signal", sig.String())
		case r := <-done:
			if m.cfg.exitOnComplete {
				if r.err != nil {
					m.exitFn(m.cfg.failureExitCode)
				} else {
					m.exitFn(m.cfg.successExitCode)
				}
			}
			return r.err
		}
	}
}

// runShutdown executes all phases. Idempotent — only runs once per Manager.
func (m *Manager) runShutdown(ctx context.Context) error {
	var aggregateErr error
	ran := false
	m.shutdownOnce.Do(func() {
		ran = true
		aggregateErr = m.executePhases(ctx)
	})
	if !ran {
		return ErrClosed
	}
	return aggregateErr
}

// executePhases runs every phase in ascending order. Within a phase
// handlers run in parallel by default unless WithSerial(phase) was set.
//
// Each phase's handlers receive a context bounded by min(handler.timeout,
// remaining global budget). After all phases run (or the budget expires),
// errors.Join is returned and OnComplete fires.
func (m *Manager) executePhases(ctx context.Context) error {
	m.markClosed()
	start := time.Now()

	// Sort handlers into phase buckets (deterministic by name within phase).
	buckets := m.bucketsByPhase()
	phases := sortedPhases(buckets)

	// Set up budget context.
	budgetCtx := ctx
	var cancelBudget context.CancelFunc
	if m.cfg.budget > 0 {
		budgetCtx, cancelBudget = context.WithTimeout(ctx, m.cfg.budget)
		defer cancelBudget()
	}

	// Watchdog: after budget+grace, force exit if still running.
	watchdogStop := m.startWatchdog(budgetCtx, start)
	defer watchdogStop()

	allErrs := []error{}
	stopped := false

	for _, p := range phases {
		if budgetCtx.Err() != nil {
			m.cfg.logger.Warn("shutdown: budget exhausted, skipping remaining phases",
				"phase", p.String(), "skipped_handlers", len(buckets[p]))
			continue
		}
		if stopped {
			break
		}

		regs := buckets[p]
		m.fireOnPhaseStart(p, len(regs))
		phaseStart := time.Now()
		phaseErrs := m.runPhase(budgetCtx, p, regs, m.cfg.serialPhases[p])
		phaseDur := time.Since(phaseStart)
		m.fireOnPhaseEnd(p, phaseDur, phaseErrs)

		if len(phaseErrs) > 0 {
			allErrs = append(allErrs, phaseErrs...)
			if m.cfg.errorPolicy == StopOnError {
				m.cfg.logger.Warn("shutdown: stopping on error per ErrorPolicy",
					"phase", p.String(), "errors", len(phaseErrs))
				stopped = true
			}
		}
	}

	totalDur := time.Since(start)
	finalErr := errors.Join(allErrs...)
	m.fireOnComplete(totalDur, finalErr)
	if finalErr != nil {
		m.cfg.logger.Error("shutdown: completed with errors", "duration", totalDur, "err", finalErr)
	} else {
		m.cfg.logger.Info("shutdown: completed cleanly", "duration", totalDur)
	}
	return finalErr
}

// markClosed flips the closed flag so subsequent Register calls fail.
func (m *Manager) markClosed() {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
}

// bucketsByPhase groups registered handlers by phase, returning a map.
// Actor interrupt handlers are placed in the actor's chosen phase as well.
func (m *Manager) bucketsByPhase() map[Phase][]registration {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make(map[Phase][]registration)
	for _, r := range m.handlers {
		out[r.phase] = append(out[r.phase], r)
	}
	for _, a := range m.actors {
		out[a.phase] = append(out[a.phase], registration{
			name:    a.name,
			fn:      a.asHandler(),
			phase:   a.phase,
			timeout: a.timeout,
		})
	}
	for phase := range out {
		sort.Slice(out[phase], func(i, j int) bool {
			return out[phase][i].name < out[phase][j].name
		})
	}
	return out
}

func sortedPhases(buckets map[Phase][]registration) []Phase {
	phases := make([]Phase, 0, len(buckets))
	for p := range buckets {
		phases = append(phases, p)
	}
	sort.Slice(phases, func(i, j int) bool { return phases[i] < phases[j] })
	return phases
}

// isShutdownSignal reports whether sig is in the configured shutdown
// signal set — i.e. should trigger phase execution rather than a custom hook.
func (m *Manager) isShutdownSignal(sig os.Signal) bool {
	for _, s := range m.cfg.signals {
		if s == sig {
			return true
		}
	}
	if len(m.cfg.signals) == 0 {
		return sig == syscall.SIGINT || sig == syscall.SIGTERM
	}
	return false
}
