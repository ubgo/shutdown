package shutdownzap

import (
	"context"
	"errors"
	"syscall"
	"testing"
	"time"

	"github.com/ubgo/shutdown"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestObserver_FullLifecycle(t *testing.T) {
	core, recorded := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)

	mgr := shutdown.New(shutdown.WithLogger(shutdown.NoopLogger()), shutdown.WithBudget(2*time.Second))
	mgr.Subscribe(Observer(logger))

	_ = mgr.Register("ok", func(_ context.Context) error { return nil })
	_ = mgr.Register("fail", func(_ context.Context) error { return errors.New("boom") })

	if err := mgr.Shutdown(context.Background()); err == nil {
		t.Fatal("expected aggregated error")
	}

	logs := recorded.All()
	want := map[string]bool{
		"shutdown: phase start":           false,
		"shutdown: phase end":             false,
		"shutdown: handler start":         false,
		"shutdown: handler end":           false,
		"shutdown: handler failed":        false,
		"shutdown: completed with errors": false,
	}
	for _, e := range logs {
		if _, ok := want[e.Message]; ok {
			want[e.Message] = true
		}
	}
	for msg, fired := range want {
		if !fired {
			t.Errorf("expected log message %q to fire", msg)
		}
	}
}

func TestObserver_OnSignalFires(t *testing.T) {
	core, recorded := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)

	obs := Observer(logger)
	obs.OnSignal(syscall.SIGTERM)

	logs := recorded.FilterMessage("shutdown: signal received").All()
	if len(logs) != 1 {
		t.Fatalf("OnSignal log count: %d", len(logs))
	}
}

func TestObserver_NilLoggerDoesNotPanic(t *testing.T) {
	obs := Observer(nil)
	obs.OnSignal(syscall.SIGTERM)
	obs.OnPhaseStart(shutdown.PhaseCloseClients, 1)
	obs.OnHandlerStart("x", shutdown.PhaseCloseClients)
	obs.OnHandlerEnd("x", shutdown.PhaseCloseClients, time.Millisecond, nil)
	obs.OnHandlerEnd("x", shutdown.PhaseCloseClients, time.Millisecond, errors.New("e"))
	obs.OnPhaseEnd(shutdown.PhaseCloseClients, time.Millisecond, nil)
	obs.OnComplete(time.Millisecond, nil)
	obs.OnComplete(time.Millisecond, errors.New("e"))
}
