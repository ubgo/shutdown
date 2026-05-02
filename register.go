package shutdown

import (
	"context"
	"time"
)

// RegisterOption configures a Register call.
type RegisterOption func(*registration)

// registration is the internal record for one registered handler.
type registration struct {
	name    string
	fn      HandlerFunc
	phase   Phase
	timeout time.Duration
}

// WithPhase places the handler in a specific phase. Default: PhaseCloseClients.
func WithPhase(p Phase) RegisterOption {
	return func(r *registration) { r.phase = p }
}

// WithTimeout caps how long this handler may run. Default is set by
// WithHandlerDefaultTimeout on the Manager (5s out of the box). The
// actual deadline is min(WithTimeout, remaining global budget).
func WithTimeout(d time.Duration) RegisterOption {
	return func(r *registration) { r.timeout = d }
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
		timeout: m.cfg.handlerDefaultTimeout,
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
//
// A panic inside the handler is converted to a wrapped error and surfaced
// to the caller — the runtime would otherwise crash the whole shutdown
// goroutine and leak the panic value, while a silent recover would hide
// the failure from the aggregated error and from the operator.
func (m *Manager) runHandler(parent context.Context, r registration) (err error) {
	deadline := r.timeout
	if deadline <= 0 {
		deadline = 5 * time.Second
	}

	hctx, cancel := context.WithTimeout(parent, deadline)
	defer cancel()

	defer func() {
		if rec := recover(); rec != nil {
			m.cfg.logger.Error("shutdown: handler panicked", "name", r.name, "panic", rec)
			err = &PanicError{Name: r.name, Value: rec}
		}
	}()

	return r.fn(hctx)
}

// PanicError wraps a recovered panic from inside a shutdown handler. The
// runner returns this in place of the handler's intended error so the
// panic surfaces in the aggregated error and observers' OnHandlerEnd hook.
type PanicError struct {
	Name  string
	Value any
}

func (e *PanicError) Error() string {
	return "shutdown: handler " + e.Name + " panicked: " + panicString(e.Value)
}

func panicString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if e, ok := v.(error); ok {
		return e.Error()
	}
	return "non-string panic value"
}
