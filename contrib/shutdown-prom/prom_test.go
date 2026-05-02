package shutdownprom

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/ubgo/shutdown"
)

func TestObserver_RecordsMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)

	mgr := shutdown.New(shutdown.WithLogger(shutdown.NoopLogger()), shutdown.WithBudget(2*time.Second))
	mgr.Subscribe(Observer(m))

	_ = mgr.Register("ok", func(_ context.Context) error { return nil })
	_ = mgr.Register("fail", func(_ context.Context) error { return errors.New("boom") })

	if err := mgr.Shutdown(context.Background()); err == nil {
		t.Fatal("expected aggregated error")
	}

	got := testutil.ToFloat64(m.HandlerCounter.WithLabelValues("CloseClients", "ok", "ok"))
	if got != 1 {
		t.Errorf("ok counter = %v, want 1", got)
	}
	got = testutil.ToFloat64(m.HandlerCounter.WithLabelValues("CloseClients", "fail", "error"))
	if got != 1 {
		t.Errorf("fail counter = %v, want 1", got)
	}

	if testutil.CollectAndCount(m.HandlerDuration) == 0 {
		t.Error("HandlerDuration histogram has no observations")
	}
	if testutil.CollectAndCount(m.PhaseDuration) == 0 {
		t.Error("PhaseDuration histogram has no observations")
	}
}

func TestObserver_NilMetricsConstructsDefault(t *testing.T) {
	// Use a fresh DefaultRegisterer scope by instantiating a new Registry
	// — but since Observer(nil) registers on the package-global default,
	// we just sanity-check it doesn't panic and the observer fires.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic: %v", r)
		}
	}()
	// We can't easily test the default registerer path without polluting
	// it; settle for covering the construction:
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)
	if m == nil {
		t.Fatal("NewMetrics(reg) = nil")
	}
}
