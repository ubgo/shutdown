// Package shutdownecho registers a shutdown handler that gracefully stops
// an Echo server.
//
// Echo's *echo.Echo type owns its server (via Start / StartServer), unlike
// gin/chi which sit on stdlib *http.Server. Echo exposes Shutdown(ctx) on
// the engine, so we use that directly.
package shutdownecho

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/ubgo/shutdown"
)

const DefaultTimeout = 10 * time.Second

type Option func(*config)

type config struct {
	name    string
	phase   shutdown.Phase
	timeout time.Duration
}

func WithName(s string) Option           { return func(c *config) { c.name = s } }
func WithPhase(p shutdown.Phase) Option  { return func(c *config) { c.phase = p } }
func WithTimeout(d time.Duration) Option { return func(c *config) { c.timeout = d } }

// Register adds a handler that calls e.Shutdown(ctx).
func Register(mgr *shutdown.Manager, e *echo.Echo, opts ...Option) error {
	cfg := &config{
		name:    "echo.Server",
		phase:   shutdown.PhaseStopAccepting,
		timeout: DefaultTimeout,
	}
	for _, o := range opts {
		o(cfg)
	}
	return mgr.Register(cfg.name, func(ctx context.Context) error {
		err := e.Shutdown(ctx)
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	},
		shutdown.WithPhase(cfg.phase),
		shutdown.WithTimeout(cfg.timeout),
	)
}
