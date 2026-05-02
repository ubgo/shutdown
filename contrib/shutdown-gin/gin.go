// Package shutdowngin registers a shutdown handler that gracefully stops
// a Gin engine wrapped in a stdlib *http.Server.
//
// Gin engines do not own a server — the user typically wraps the engine
// in a *http.Server they control. This package therefore takes the same
// *http.Server you Listen on, ensuring the same drain semantics as the
// nethttp adapter. The Gin engine itself does not need to be passed in.
//
// If you use gin.Engine.Run() rather than your own server, switch to:
//
//	srv := &http.Server{Handler: engine}
//	go srv.ListenAndServe()
//	shutdowngin.Register(mgr, srv)
package shutdowngin

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/ubgo/shutdown"
)

// DefaultTimeout is the default per-handler timeout for the Gin server's
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
// Default: "gin.Server".
func WithName(s string) Option { return func(c *config) { c.name = s } }

// WithPhase places the Gin shutdown in a specific phase.
// Default: shutdown.PhaseStopAccepting.
func WithPhase(p shutdown.Phase) Option { return func(c *config) { c.phase = p } }

// WithTimeout caps the time spent inside srv.Shutdown.
// Default: DefaultTimeout.
func WithTimeout(d time.Duration) Option {
	return func(c *config) { c.timeout = d }
}

// Register adds a handler that calls srv.Shutdown(ctx). srv is the
// *http.Server wrapping your gin.Engine.
func Register(mgr *shutdown.Manager, srv *http.Server, opts ...Option) error {
	cfg := &config{
		name:    "gin.Server",
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
