package shutdownfiber

import (
	"context"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ubgo/shutdown"
)

func TestRegister_GracefulShutdown(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})

	mgr := shutdown.New(shutdown.WithLogger(shutdown.NoopLogger()), shutdown.WithBudget(2*time.Second))
	if err := Register(mgr, app); err != nil {
		t.Fatalf("Register: %v", err)
	}
	// App not started — Shutdown is a no-op success.
	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestRegister_AllOptions(t *testing.T) {
	mgr := shutdown.New(shutdown.WithLogger(shutdown.NoopLogger()))
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	if err := Register(mgr, app,
		WithName("fiber-api"),
		WithPhase(shutdown.PhaseStopAccepting),
		WithTimeout(time.Second),
	); err != nil {
		t.Fatalf("Register: %v", err)
	}
}
