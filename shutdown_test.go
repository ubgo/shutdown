package shutdown

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeHandler builds a HandlerFunc that records its invocation timestamp
// and returns the given error after the given delay.
func fakeHandler(t *testing.T, calls *[]string, name string, dur time.Duration, err error) HandlerFunc {
	t.Helper()
	return func(ctx context.Context) error {
		select {
		case <-time.After(dur):
		case <-ctx.Done():
			return ctx.Err()
		}
		*calls = append(*calls, name)
		return err
	}
}

func TestRegister_RejectsEmptyName(t *testing.T) {
	m := New()
	if err := m.Register("", fakeHandler(t, &[]string{}, "x", 0, nil)); !errors.Is(err, ErrEmptyName) {
		t.Errorf("Register empty name: got %v, want ErrEmptyName", err)
	}
}

func TestRegister_RejectsDuplicate(t *testing.T) {
	m := New()
	_ = m.Register("a", fakeHandler(t, &[]string{}, "a", 0, nil))
	if err := m.Register("a", fakeHandler(t, &[]string{}, "a", 0, nil)); !errors.Is(err, ErrAlreadyRegistered) {
		t.Errorf("Register duplicate: got %v, want ErrAlreadyRegistered", err)
	}
}

func TestRegister_RejectsNilFn(t *testing.T) {
	m := New()
	if err := m.Register("a", nil); err == nil {
		t.Errorf("Register nil fn: got nil, want error")
	}
}

func TestShutdown_PhaseOrdering(t *testing.T) {
	m := New(WithLogger(NoopLogger()), WithBudget(2*time.Second))

	var mu sync.Mutex
	var order []string
	rec := func(name string) HandlerFunc {
		return func(_ context.Context) error {
			mu.Lock()
			order = append(order, name)
			mu.Unlock()
			return nil
		}
	}

	_ = m.Register("close-clients", rec("close-clients"), WithPhase(PhaseCloseClients))
	_ = m.Register("flush-logs", rec("flush-logs"), WithPhase(PhaseFlushLogs))
	_ = m.Register("stop-accepting", rec("stop-accepting"), WithPhase(PhaseStopAccepting))

	if err := m.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	want := []string{"stop-accepting", "close-clients", "flush-logs"}
	if len(order) != len(want) {
		t.Fatalf("order length: got %d, want %d (%v)", len(order), len(want), order)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("order[%d]: got %q, want %q (full: %v)", i, order[i], want[i], order)
		}
	}
}

func TestShutdown_ParallelWithinPhase(t *testing.T) {
	m := New(WithLogger(NoopLogger()), WithBudget(2*time.Second))

	delay := 100 * time.Millisecond
	calls := []string{}
	var mu sync.Mutex
	mk := func(name string) HandlerFunc {
		return func(_ context.Context) error {
			time.Sleep(delay)
			mu.Lock()
			calls = append(calls, name)
			mu.Unlock()
			return nil
		}
	}
	for _, n := range []string{"a", "b", "c"} {
		_ = m.Register(n, mk(n), WithPhase(PhaseCloseClients))
	}

	start := time.Now()
	_ = m.Shutdown(context.Background())
	elapsed := time.Since(start)

	if elapsed > 250*time.Millisecond {
		t.Errorf("expected parallel execution (~100ms), got %v", elapsed)
	}
	if len(calls) != 3 {
		t.Errorf("calls: got %d, want 3", len(calls))
	}
}

func TestShutdown_SerialWithinPhase(t *testing.T) {
	m := New(WithLogger(NoopLogger()), WithBudget(2*time.Second), WithSerial(PhaseCloseClients))

	delay := 60 * time.Millisecond
	mk := func() HandlerFunc {
		return func(_ context.Context) error {
			time.Sleep(delay)
			return nil
		}
	}
	for _, n := range []string{"a", "b", "c"} {
		_ = m.Register(n, mk(), WithPhase(PhaseCloseClients))
	}

	start := time.Now()
	_ = m.Shutdown(context.Background())
	elapsed := time.Since(start)

	if elapsed < 3*delay {
		t.Errorf("expected serial execution (~%v), got %v", 3*delay, elapsed)
	}
}

func TestShutdown_PerHandlerTimeout(t *testing.T) {
	m := New(WithLogger(NoopLogger()), WithBudget(2*time.Second))

	_ = m.Register("slow", func(ctx context.Context) error {
		select {
		case <-time.After(500 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}, WithTimeout(50*time.Millisecond), WithPhase(PhaseCloseClients))

	err := m.Shutdown(context.Background())
	if err == nil {
		t.Fatalf("expected aggregated timeout error, got nil")
	}
}

func TestShutdown_ContinueOnError_Aggregates(t *testing.T) {
	m := New(WithLogger(NoopLogger()), WithBudget(2*time.Second))

	_ = m.Register("first", func(_ context.Context) error { return errors.New("first-err") },
		WithPhase(PhaseCloseClients))
	_ = m.Register("second", func(_ context.Context) error { return errors.New("second-err") },
		WithPhase(PhaseCloseClients))

	err := m.Shutdown(context.Background())
	if err == nil || !contains(err.Error(), "first-err") || !contains(err.Error(), "second-err") {
		t.Errorf("expected joined error containing both, got %v", err)
	}
}

func TestShutdown_StopOnError_HaltsAtFirstFailure(t *testing.T) {
	m := New(
		WithLogger(NoopLogger()),
		WithBudget(2*time.Second),
		WithErrorPolicy(StopOnError),
	)

	var ranAfterFailure atomic.Int32
	_ = m.Register("phase1-fail", func(_ context.Context) error { return errors.New("nope") },
		WithPhase(PhaseStopAccepting))
	_ = m.Register("phase2-should-skip", func(_ context.Context) error {
		ranAfterFailure.Add(1)
		return nil
	}, WithPhase(PhaseCloseClients))

	_ = m.Shutdown(context.Background())
	if ranAfterFailure.Load() != 0 {
		t.Errorf("StopOnError: phase2 ran despite phase1 failing")
	}
}

func TestShutdown_Idempotent(t *testing.T) {
	m := New(WithLogger(NoopLogger()), WithBudget(2*time.Second))
	calls := atomic.Int32{}
	_ = m.Register("h", func(_ context.Context) error {
		calls.Add(1)
		return nil
	})

	_ = m.Shutdown(context.Background())
	if err := m.Shutdown(context.Background()); !errors.Is(err, ErrClosed) {
		t.Errorf("second Shutdown: got %v, want ErrClosed", err)
	}
	if calls.Load() != 1 {
		t.Errorf("handler ran %d times, want 1", calls.Load())
	}
}

func TestShutdown_ObserverCallbacks(t *testing.T) {
	m := New(WithLogger(NoopLogger()), WithBudget(2*time.Second))

	var (
		mu      sync.Mutex
		signals []string
		hStart  []string
		hEnd    []string
		pStart  []string
		pEnd    []string
		done    int32
	)
	m.Subscribe(Observer{
		OnHandlerStart: func(name string, _ Phase) {
			mu.Lock()
			hStart = append(hStart, name)
			mu.Unlock()
		},
		OnHandlerEnd: func(name string, _ Phase, _ time.Duration, _ error) {
			mu.Lock()
			hEnd = append(hEnd, name)
			mu.Unlock()
		},
		OnPhaseStart: func(p Phase, _ int) {
			mu.Lock()
			pStart = append(pStart, p.String())
			mu.Unlock()
		},
		OnPhaseEnd: func(p Phase, _ time.Duration, _ []error) {
			mu.Lock()
			pEnd = append(pEnd, p.String())
			mu.Unlock()
		},
		OnComplete: func(_ time.Duration, _ error) {
			atomic.AddInt32(&done, 1)
		},
	})

	_ = m.Register("a", func(_ context.Context) error { return nil }, WithPhase(PhaseCloseClients))
	_ = m.Register("b", func(_ context.Context) error { return nil }, WithPhase(PhaseCloseClients))

	_ = m.Shutdown(context.Background())

	_ = signals // not exercised here
	if len(hStart) != 2 || len(hEnd) != 2 {
		t.Errorf("handler hooks: start=%v end=%v", hStart, hEnd)
	}
	if len(pStart) != 1 || len(pEnd) != 1 {
		t.Errorf("phase hooks: start=%v end=%v", pStart, pEnd)
	}
	if atomic.LoadInt32(&done) != 1 {
		t.Errorf("OnComplete fired %d times, want 1", done)
	}
}

func TestShutdown_HandlerPanic_DoesNotBubble(t *testing.T) {
	t.Helper()
	m := New(WithLogger(NoopLogger()), WithBudget(2*time.Second))
	_ = m.Register("panicky", func(_ context.Context) error {
		panic("boom")
	})
	// Run shutdown — must not propagate the panic.
	_ = m.Shutdown(context.Background())
}

func TestRegister_AfterShutdown_Fails(t *testing.T) {
	m := New(WithLogger(NoopLogger()), WithBudget(2*time.Second))
	_ = m.Register("a", func(_ context.Context) error { return nil })
	_ = m.Shutdown(context.Background())

	if err := m.Register("b", func(_ context.Context) error { return nil }); !errors.Is(err, ErrClosed) {
		t.Errorf("Register after shutdown: got %v, want ErrClosed", err)
	}
}

func TestActor_RegisterAndDone(t *testing.T) {
	m := New(WithLogger(NoopLogger()), WithBudget(2*time.Second))
	interrupted := atomic.Int32{}

	handle, err := m.RegisterActor("worker", func(_ error) {
		interrupted.Add(1)
	})
	if err != nil {
		t.Fatalf("RegisterActor: %v", err)
	}

	go func() {
		// Simulate run loop exiting once interrupt is invoked.
		for interrupted.Load() == 0 {
			time.Sleep(5 * time.Millisecond)
		}
		handle.Done(nil)
	}()

	_ = m.Shutdown(context.Background())
	if interrupted.Load() != 1 {
		t.Errorf("interrupt not called: got %d", interrupted.Load())
	}
}

func TestActor_TimeoutWhenDoneNeverCalled(t *testing.T) {
	m := New(WithLogger(NoopLogger()), WithBudget(2*time.Second))
	_, err := m.RegisterActor("stuck", func(_ error) {}, WithActorTimeout(50*time.Millisecond))
	if err != nil {
		t.Fatalf("RegisterActor: %v", err)
	}

	err = m.Shutdown(context.Background())
	if err == nil {
		t.Errorf("expected timeout error from stuck actor, got nil")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
