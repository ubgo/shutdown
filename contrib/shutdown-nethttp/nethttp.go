// Package shutdownnethttp registers a shutdown handler that gracefully
// stops a stdlib *http.Server.
//
// The handler is placed in PhaseStopAccepting by default — the right phase
// for the listener half of a service: stop accepting new connections so
// in-flight requests can drain in PhaseDrainTraffic afterwards.
//
// The server's Shutdown(ctx) is called with the per-handler ctx. If
// Shutdown returns an error other than http.ErrServerClosed it is
// propagated to the manager and ends up in the aggregated error.
package shutdownnethttp

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/ubgo/shutdown"
)

// Default per-handler timeout for HTTP graceful shutdown. Net/http servers
// usually finish their drain in well under this; the per-call value is
// still capped by the manager's global budget.
const DefaultTimeout = 10 * time.Second

// Option configures a Register call.
type Option func(*config)

type config struct {
	name    string
	phase   shutdown.Phase
	timeout time.Duration
}

// WithName overrides the handler name registered with the manager.
// Default: "http.Server".
func WithName(s string) Option {
	return func(c *config) { c.name = s }
}

// WithPhase places the HTTP shutdown in a specific phase.
// Default: shutdown.PhaseStopAccepting.
func WithPhase(p shutdown.Phase) Option {
	return func(c *config) { c.phase = p }
}

// WithTimeout caps the time spent inside srv.Shutdown.
// Default: DefaultTimeout.
func WithTimeout(d time.Duration) Option {
	return func(c *config) { c.timeout = d }
}

// Register adds a handler to mgr that calls srv.Shutdown(ctx) when the
// configured phase fires.
//
// Returns an error if the manager rejects the registration (duplicate
// name, manager already closed, etc).
func Register(mgr *shutdown.Manager, srv *http.Server, opts ...Option) error {
	cfg := &config{
		name:    "http.Server",
		phase:   shutdown.PhaseStopAccepting,
		timeout: DefaultTimeout,
	}
	for _, o := range opts {
		o(cfg)
	}

	return mgr.Register(cfg.name, func(ctx context.Context) error {
		err := srv.Shutdown(ctx)
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	},
		shutdown.WithPhase(cfg.phase),
		shutdown.WithTimeout(cfg.timeout),
	)
}
