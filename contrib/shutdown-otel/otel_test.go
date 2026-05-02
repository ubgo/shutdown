package shutdownotel

import (
	"context"
	"errors"
	"syscall"
	"testing"
	"time"

	"github.com/ubgo/shutdown"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func newTestTracer() (trace.Tracer, *tracetest.SpanRecorder) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	return tp.Tracer("shutdown-otel-test"), rec
}

func TestObserver_RecordsRootPhaseHandlerSpans(t *testing.T) {
	tracer, rec := newTestTracer()

	mgr := shutdown.New(shutdown.WithLogger(shutdown.NoopLogger()), shutdown.WithBudget(2*time.Second))
	mgr.Subscribe(Observer(tracer))

	_ = mgr.Register("a", func(_ context.Context) error { return nil })

	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	spans := rec.Ended()
	names := map[string]int{}
	for _, s := range spans {
		names[s.Name()]++
	}
	if names["shutdown"] == 0 {
		t.Error("missing root span 'shutdown'")
	}
	if names["shutdown.phase.CloseClients"] == 0 {
		t.Error("missing phase span")
	}
	if names["shutdown.handler.a"] == 0 {
		t.Error("missing handler span")
	}
}

func TestObserver_HandlerErrorRecorded(t *testing.T) {
	tracer, rec := newTestTracer()

	mgr := shutdown.New(shutdown.WithLogger(shutdown.NoopLogger()), shutdown.WithBudget(2*time.Second))
	mgr.Subscribe(Observer(tracer))

	_ = mgr.Register("fail", func(_ context.Context) error { return errors.New("boom") })

	if err := mgr.Shutdown(context.Background()); err == nil {
		t.Fatal("expected error")
	}

	var found bool
	for _, s := range rec.Ended() {
		if s.Name() == "shutdown.handler.fail" {
			found = true
			if len(s.Events()) == 0 {
				t.Error("expected RecordError event on handler span")
			}
		}
	}
	if !found {
		t.Error("handler span not found")
	}
}

func TestObserver_OnSignalEvent(t *testing.T) {
	tracer, rec := newTestTracer()
	obs := Observer(tracer)

	obs.OnSignal(syscall.SIGTERM)
	obs.OnComplete(10*time.Millisecond, nil)

	for _, s := range rec.Ended() {
		if s.Name() == "shutdown" {
			if len(s.Events()) == 0 {
				t.Error("expected signal event on root span")
			}
			return
		}
	}
	t.Error("root span not closed")
}

func TestObserver_NilTracerDoesNotPanic(t *testing.T) {
	obs := Observer(nil)
	obs.OnSignal(syscall.SIGTERM)
	obs.OnPhaseStart(shutdown.PhaseCloseClients, 1)
	obs.OnHandlerStart("x", shutdown.PhaseCloseClients)
	obs.OnHandlerEnd("x", shutdown.PhaseCloseClients, time.Millisecond, errors.New("e"))
	obs.OnPhaseEnd(shutdown.PhaseCloseClients, time.Millisecond, []error{errors.New("e")})
	obs.OnComplete(time.Millisecond, errors.New("done"))
}
