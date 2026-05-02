// Package shutdownfiber registers a shutdown handler that gracefully
// stops a Fiber app.
//
// Fiber owns its own server (built on fasthttp); use ShutdownWithContext
// where available so the per-handler context flows through.
package shutdownfiber

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
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

// Register adds a handler that calls app.ShutdownWithContext(ctx).
func Register(mgr *shutdown.Manager, app *fiber.App, opts ...Option) error {
	cfg := &config{
		name:    "fiber.App",
		phase:   shutdown.PhaseStopAccepting,
		timeout: DefaultTimeout,
	}
	for _, o := range opts {
		o(cfg)
	}
	return mgr.Register(cfg.name, func(ctx context.Context) error {
		return app.ShutdownWithContext(ctx)
	},
		shutdown.WithPhase(cfg.phase),
		shutdown.WithTimeout(cfg.timeout),
	)
}
