package shutdown

import (
	"os"
	"syscall"
	"time"
)

// Option configures a Manager.
type Option func(*config)

// config is the internal Manager configuration. All fields populated by
// New from defaultConfig and any user-supplied Options.
type config struct {
	logger              Logger
	signals             []os.Signal
	budget              time.Duration
	errorPolicy         ErrorPolicy
	forceOnSecondSignal bool
	forceExitCode       int
	exitOnComplete      bool
	successExitCode     int
	failureExitCode     int
	serialPhases        map[Phase]bool

	// Test seam. Defaults to os.Exit; tests inject a recording function.
	exitFn func(code int)

	// watchdog grace beyond budget before hard-exit.
	watchdogGrace time.Duration
}

func defaultConfig() config {
	return config{
		signals:             []os.Signal{syscall.SIGINT, syscall.SIGTERM},
		budget:              30 * time.Second,
		errorPolicy:         ContinueOnError,
		forceOnSecondSignal: true,
		forceExitCode:       130, // 128 + SIGINT, the conventional shell code
		exitOnComplete:      false,
		successExitCode:     0,
		failureExitCode:     1,
		serialPhases:        map[Phase]bool{},
		exitFn:              os.Exit,
		watchdogGrace:       1 * time.Second,
	}
}

// WithLogger overrides the default slog-backed logger.
func WithLogger(l Logger) Option {
	return func(c *config) { c.logger = l }
}

// WithSignals overrides the listened signal set. Default: SIGINT, SIGTERM.
//
// If you want to add a non-shutdown signal hook (e.g. SIGHUP for reload),
// use Manager.OnSignal instead — that registers a hook without making the
// signal trigger a shutdown.
func WithSignals(sigs ...os.Signal) Option {
	return func(c *config) {
		c.signals = append([]os.Signal{}, sigs...)
	}
}

// WithBudget sets the total wall-clock budget across all phases. After the
// budget expires, in-flight handler contexts are cancelled and the watchdog
// hard-exits the process after a 1-second grace period (configurable via
// WithWatchdogGrace). Default: 30s.
func WithBudget(d time.Duration) Option {
	return func(c *config) { c.budget = d }
}

// WithWatchdogGrace sets the grace period after the budget expires before
// the watchdog calls os.Exit. Default: 1s.
func WithWatchdogGrace(d time.Duration) Option {
	return func(c *config) { c.watchdogGrace = d }
}

// WithErrorPolicy overrides the ContinueOnError default.
func WithErrorPolicy(p ErrorPolicy) Option {
	return func(c *config) { c.errorPolicy = p }
}

// WithForceOnSecondSignal makes a second signal during shutdown trigger an
// immediate os.Exit(forceCode). Default: true with forceCode=130.
//
// Set enabled=false to ignore second signals (the orchestrator's SIGKILL
// is then the only escape hatch — useful only when you trust the watchdog
// budget completely).
func WithForceOnSecondSignal(enabled bool, forceCode int) Option {
	return func(c *config) {
		c.forceOnSecondSignal = enabled
		c.forceExitCode = forceCode
	}
}

// WithExitOnComplete makes Listen call os.Exit at the end of shutdown.
// successCode is used when the aggregated error is nil; failureCode
// otherwise. Default: never exit (just return).
func WithExitOnComplete(successCode, failureCode int) Option {
	return func(c *config) {
		c.exitOnComplete = true
		c.successExitCode = successCode
		c.failureExitCode = failureCode
	}
}

// WithSerial(phase) opts a specific phase out of parallel handler execution.
// By default all handlers in a phase run in parallel.
func WithSerial(phase Phase) Option {
	return func(c *config) { c.serialPhases[phase] = true }
}

// withExitFn is an internal test seam to inject a recording exit function
// instead of calling os.Exit. Not exported.
func withExitFn(fn func(int)) Option {
	return func(c *config) { c.exitFn = fn }
}

// exitFn is the manager's shorthand for c.exitFn or os.Exit.
func (m *Manager) exitFn(code int) {
	if m.cfg.exitFn != nil {
		m.cfg.exitFn(code)
		return
	}
	os.Exit(code)
}
