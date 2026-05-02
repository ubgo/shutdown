package shutdownecho

import (
	"context"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/ubgo/shutdown"
)

func TestRegister_GracefulShutdown(t *testing.T) {
	e := echo.New()
	e.HideBanner = true

	mgr := shutdown.New(shutdown.WithLogger(shutdown.NoopLogger()), shutdown.WithBudget(2*time.Second))
	if err := Register(mgr, e); err != nil {
		t.Fatalf("Register: %v", err)
	}
	// Echo not started — Shutdown is a no-op success.
	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestRegister_AllOptions(t *testing.T) {
	mgr := shutdown.New(shutdown.WithLogger(shutdown.NoopLogger()))
	e := echo.New()
	if err := Register(mgr, e,
		WithName("echo-api"),
		WithPhase(shutdown.PhaseStopAccepting),
		WithTimeout(time.Second),
	); err != nil {
		t.Fatalf("Register: %v", err)
	}
}
