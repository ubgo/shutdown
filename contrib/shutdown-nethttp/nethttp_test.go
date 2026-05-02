package shutdownnethttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ubgo/shutdown"
)

func TestRegister_GracefulShutdown(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.Start()
	t.Cleanup(srv.Close)

	mgr := shutdown.New(
		shutdown.WithLogger(shutdown.NoopLogger()),
		shutdown.WithBudget(2*time.Second),
	)

	if err := Register(mgr, srv.Config); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Confirm server is up.
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("pre-shutdown GET: %v", err)
	}
	_ = resp.Body.Close()

	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	// After shutdown, new requests should fail.
	if _, err := http.Get(srv.URL); err == nil {
		t.Error("expected GET to fail after shutdown")
	}
}

func TestRegister_PhaseAndNameOverride(t *testing.T) {
	srv := &http.Server{Addr: ":0"}
	mgr := shutdown.New(shutdown.WithLogger(shutdown.NoopLogger()))
	if err := Register(mgr, srv,
		WithName("api"),
		WithPhase(shutdown.PhaseFlushQueues),
		WithTimeout(time.Second),
	); err != nil {
		t.Fatalf("Register: %v", err)
	}
	// Re-registering with same name should fail.
	if err := Register(mgr, srv, WithName("api")); !errors.Is(err, shutdown.ErrAlreadyRegistered) {
		t.Errorf("duplicate name: got %v, want ErrAlreadyRegistered", err)
	}
}

func TestRegister_PropagatesNonClosedError(t *testing.T) {
	mgr := shutdown.New(shutdown.WithLogger(shutdown.NoopLogger()))
	srv := &http.Server{}
	if err := Register(mgr, srv, WithTimeout(20*time.Millisecond)); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Server was never started, so Shutdown returns nil immediately — no
	// error to aggregate.
	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown of unused server: got %v, want nil", err)
	}
}
