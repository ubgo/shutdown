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

const DefaultTimeout = 10 * time.Second

type Option func(*config)

type config struct {
	name    string
	phase   shutdown.Phase
	timeout time.Duration
}

func WithName(s string) Option          { return func(c *config) { c.name = s } }
func WithPhase(p shutdown.Phase) Option { return func(c *config) { c.phase = p } }
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
