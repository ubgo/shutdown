package shutdownchi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ubgo/shutdown"
)

func TestRegister_GracefulShutdown(t *testing.T) {
	r := chi.NewRouter()
	r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := httptest.NewUnstartedServer(r)
	srv.Start()
	t.Cleanup(srv.Close)

	mgr := shutdown.New(shutdown.WithLogger(shutdown.NoopLogger()), shutdown.WithBudget(2*time.Second))
	if err := Register(mgr, srv.Config); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestRegister_AllOptions(t *testing.T) {
	mgr := shutdown.New(shutdown.WithLogger(shutdown.NoopLogger()))
	srv := &http.Server{}
	if err := Register(mgr, srv,
		WithName("chi-api"),
		WithPhase(shutdown.PhaseStopAccepting),
		WithTimeout(time.Second),
	); err != nil {
		t.Fatalf("Register: %v", err)
	}
}
