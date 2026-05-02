package shutdown

import (
	"context"
	"time"
)

// RegisterOption configures a Register call.
type RegisterOption func(*registration)

// registration is the internal record for one registered handler.
type registration struct {
	name     string
	fn       HandlerFunc
	phase    Phase
	timeout  time.Duration
	required bool
}

// WithPhase places the handler in a specific phase. Default: PhaseCloseClients.
func WithPhase(p Phase) RegisterOption {
	return func(r *registration) { r.phase = p }
}

// WithTimeout caps how long this handler may run. Default: 5s. The actual
// deadline is min(WithTimeout, remaining global budget).
func WithTimeout(d time.Duration) RegisterOption {
	return func(r *registration) { r.timeout = d }
}

// WithRequired marks the handler as required. Required handlers' errors
// always land in the aggregated return value regardless of ErrorPolicy;
// non-required handlers' errors are still logged but only land in the
// aggregate when ErrorPolicy is ContinueOnError (the default).
func WithRequired() RegisterOption {
	return func(r *registration) { r.required = true }
}

// Register adds a shutdown handler. Returns an error if name is empty,
// already registered, or the Manager has already started shutting down.
func (m *Manager) Register(name string, fn HandlerFunc, opts ...RegisterOption) error {
	if name == "" {
		return ErrEmptyName
	}
	if fn == nil {
		return errNilHandler
	}

	r := registration{
		name:    name,
		fn:      fn,
		phase:   PhaseCloseClients,
		timeout: 5 * time.Second,
	}
	for _, o := range opts {
		o(&r)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrClosed
	}
	if _, exists := m.handlers[name]; exists {
		return ErrAlreadyRegistered
	}
	m.handlers[name] = r
	return nil
}

// errNilHandler is returned when Register is called with a nil function.
var errNilHandler = errSentinel("shutdown: handler function must not be nil")

// errSentinel is a tiny string-backed error type so we can declare more
// sentinel errors without importing fmt/errors at every call.
type errSentinel string

func (e errSentinel) Error() string { return string(e) }

// runHandler runs one handler with the per-handler + budget timeout.
// Used by the runner to ensure consistent ctx-derivation logic.
func (m *Manager) runHandler(parent context.Context, r registration) error {
	deadline := r.timeout
	if deadline <= 0 {
		deadline = 5 * time.Second
	}

	// Bound by parent (which already carries the global budget).
	hctx, cancel := context.WithTimeout(parent, deadline)
	defer cancel()

	defer func() {
		if rec := recover(); rec != nil {
			m.cfg.logger.Error("shutdown: handler panicked", "name", r.name, "panic", rec)
		}
	}()

	return r.fn(hctx)
}
