// Package shutdownchi registers a shutdown handler that gracefully stops
// a stdlib *http.Server wrapping a chi.Router.
//
// Chi sits on net/http directly, so this package's surface is identical
// to shutdownnethttp — it exists for naming symmetry with the rest of the
// HTTP framework family and so users discover it by pkg name.
package shutdownchi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/ubgo/shutdown"
)

// DefaultTimeout is the default per-handler timeout for the Chi server's
// graceful shutdown.
const DefaultTimeout = 10 * time.Second

// Option configures Register.
type Option func(*config)

type config struct {
	name    string
	phase   shutdown.Phase
	timeout time.Duration
}

// WithName overrides the handler name registered with the manager.
// Default: "chi.Server".
func WithName(s string) Option { return func(c *config) { c.name = s } }

// WithPhase places the Chi shutdown in a specific phase.
// Default: shutdown.PhaseStopAccepting.
func WithPhase(p shutdown.Phase) Option { return func(c *config) { c.phase = p } }

// WithTimeout caps the time spent inside srv.Shutdown.
// Default: DefaultTimeout.
func WithTimeout(d time.Duration) Option { return func(c *config) { c.timeout = d } }

// Register adds a handler that calls srv.Shutdown(ctx). srv is the
// *http.Server wrapping your chi.Router.
func Register(mgr *shutdown.Manager, srv *http.Server, opts ...Option) error {
	cfg := &config{
		name:    "chi.Server",
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
