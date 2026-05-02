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

// DefaultTimeout is the default per-handler timeout for Echo's graceful shutdown.
const DefaultTimeout = 10 * time.Second

// Option configures Register.
type Option func(*config)

type config struct {
	name    string
	phase   shutdown.Phase
	timeout time.Duration
}

// WithName overrides the handler name registered with the manager.
// Default: "echo.Server".
func WithName(s string) Option { return func(c *config) { c.name = s } }

// WithPhase places the Echo shutdown in a specific phase.
// Default: shutdown.PhaseStopAccepting.
func WithPhase(p shutdown.Phase) Option { return func(c *config) { c.phase = p } }

// WithTimeout caps the time spent inside e.Shutdown.
// Default: DefaultTimeout.
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
