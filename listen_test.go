package shutdown

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestListen_ReturnsOnCtxCancel(t *testing.T) {
	m := New(WithLogger(NoopLogger()), WithBudget(2*time.Second))

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	err := m.Listen(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Listen ctx cancel: got %v, want context.Canceled", err)
	}
}

func TestListen_RunsShutdownOnSignal(t *testing.T) {
	m := New(WithLogger(NoopLogger()), WithBudget(1*time.Second))

	ran := atomic.Int32{}
	_ = m.Register("h", func(_ context.Context) error {
		ran.Add(1)
		return nil
	})

	ctx := context.Background()
	done := make(chan error, 1)
	go func() { done <- m.Listen(ctx) }()

	// Give Listen a moment to install signal handler.
	time.Sleep(20 * time.Millisecond)

	// Send our own SIGTERM (only this process — using syscall.SIGTERM
	// to ourselves is the standard test technique for signal handlers).
	p, _ := os.FindProcess(os.Getpid())
	_ = p.Signal(syscall.SIGTERM)

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Listen returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Listen didn't return after signal")
	}

	if ran.Load() != 1 {
		t.Errorf("handler ran %d times, want 1", ran.Load())
	}
}

func TestListen_ForceExitOnSecondSignal(t *testing.T) {
	exitCalls := atomic.Int32{}
	exitedWith := atomic.Int32{}
	exitFn := func(code int) {
		exitCalls.Add(1)
		exitedWith.Store(int32(code))
	}

	m := New(
		WithLogger(NoopLogger()),
		WithBudget(5*time.Second),
		withExitFn(exitFn),
	)

	// Register a handler that takes a long time (simulate hang).
	_ = m.Register("hang", func(ctx context.Context) error {
		select {
		case <-time.After(3 * time.Second):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}, WithTimeout(3*time.Second))

	ctx := context.Background()
	done := make(chan error, 1)
	go func() { done <- m.Listen(ctx) }()

	time.Sleep(20 * time.Millisecond)

	p, _ := os.FindProcess(os.Getpid())
	_ = p.Signal(syscall.SIGTERM)

	time.Sleep(50 * time.Millisecond)
	_ = p.Signal(syscall.SIGTERM) // second signal — should trigger force exit

	// Listen returns when force-exit happens (since we injected exitFn,
	// the fake doesn't actually exit).
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("Listen did not return after force-exit was triggered")
	}

	if exitCalls.Load() == 0 {
		t.Errorf("expected exitFn to be called for force-exit")
	}
	if exitedWith.Load() != 130 {
		t.Errorf("force exit code: got %d, want 130", exitedWith.Load())
	}
}

func TestListen_OnSignalAutoIncludesUnlistedSignal(t *testing.T) {
	// Register OnSignal for SIGUSR2 without including it in WithSignals —
	// the manager must add it to the listened set automatically so the hook
	// actually fires.
	m := New(WithLogger(NoopLogger()), WithBudget(1*time.Second))

	hookFired := atomic.Int32{}
	m.OnSignal(syscall.SIGUSR2, func(_ context.Context, _ os.Signal) {
		hookFired.Add(1)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- m.Listen(ctx) }()

	time.Sleep(20 * time.Millisecond)
	p, _ := os.FindProcess(os.Getpid())
	_ = p.Signal(syscall.SIGUSR2)

	err := <-done
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Errorf("Listen: got %v, want ctx error", err)
	}
	if hookFired.Load() == 0 {
		t.Fatal("OnSignal hook for unlisted signal did not fire — auto-include broken")
	}
}

func TestListen_OnSignalHookFiresWithoutShutdown(t *testing.T) {
	m := New(WithLogger(NoopLogger()), WithBudget(1*time.Second))

	hookFired := atomic.Int32{}
	m.OnSignal(syscall.SIGUSR1, func(_ context.Context, _ os.Signal) {
		hookFired.Add(1)
	})

	// Add SIGUSR1 to the listened set (otherwise the OS doesn't deliver
	// it to us through signal.Notify).
	m2 := New(WithLogger(NoopLogger()), WithBudget(1*time.Second),
		WithSignals(syscall.SIGUSR1, syscall.SIGTERM))
	m2.OnSignal(syscall.SIGUSR1, func(_ context.Context, _ os.Signal) {
		hookFired.Add(1)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- m2.Listen(ctx) }()

	time.Sleep(20 * time.Millisecond)
	p, _ := os.FindProcess(os.Getpid())
	_ = p.Signal(syscall.SIGUSR1)

	// Listen continues after the hook fires; ctx will eventually cancel it.
	err := <-done
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Errorf("Listen: got %v, want ctx error", err)
	}
	if hookFired.Load() == 0 {
		t.Errorf("OnSignal hook did not fire")
	}
}

func TestWatchdog_ForceExitsOnBudgetOverrun(t *testing.T) {
	exitCalls := atomic.Int32{}
	exitFn := func(_ int) { exitCalls.Add(1) }

	m := New(
		WithLogger(NoopLogger()),
		WithBudget(50*time.Millisecond),
		WithWatchdogGrace(20*time.Millisecond),
		withExitFn(exitFn),
	)
	// Handler that ignores ctx for longer than budget+grace.
	_ = m.Register("hang", func(_ context.Context) error {
		time.Sleep(300 * time.Millisecond)
		return nil
	}, WithTimeout(500*time.Millisecond))

	_ = m.Shutdown(context.Background())

	if exitCalls.Load() == 0 {
		t.Errorf("watchdog did not fire force-exit")
	}
}

func TestWithHandlerDefaultTimeout_OverridesBaseline(t *testing.T) {
	m := New(
		WithLogger(NoopLogger()),
		WithBudget(2*time.Second),
		WithHandlerDefaultTimeout(40*time.Millisecond),
	)
	// Handler that respects ctx — should be cancelled by the default timeout
	// (40ms) before its 500ms work completes.
	if err := m.Register("slow", func(ctx context.Context) error {
		select {
		case <-time.After(500 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	start := time.Now()
	err := m.Shutdown(context.Background())
	if err == nil {
		t.Fatal("expected timeout error from default handler timeout")
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Errorf("WithHandlerDefaultTimeout did not apply: shutdown took %v", elapsed)
	}
}

func TestPhase_StringCoverage(t *testing.T) {
	cases := map[Phase]string{
		PhasePreShutdown:   "PreShutdown",
		PhaseStopAccepting: "StopAccepting",
		PhaseDrainTraffic:  "DrainTraffic",
		PhaseFlushQueues:   "FlushQueues",
		PhaseCloseClients:  "CloseClients",
		PhaseFlushLogs:     "FlushLogs",
		PhasePostShutdown:  "PostShutdown",
		Phase(42):          "phase=42",
		Phase(-7):          "phase=-7",
	}
	for p, want := range cases {
		if got := p.String(); got != want {
			t.Errorf("Phase(%d).String(): got %q, want %q", int(p), got, want)
		}
	}
}

func TestLogger_DefaultAndAdapters(t *testing.T) {
	// defaultLogger (slog) — fire each method to exercise.
	l := defaultLogger()
	l.Info("info-msg", "k", "v")
	l.Warn("warn-msg")
	l.Error("error-msg", "err", errors.New("test"))

	// NoopLogger — no panic.
	n := NoopLogger()
	n.Info("x")
	n.Warn("x")
	n.Error("x")

	// SlogLogger with explicit logger.
	sl := SlogLogger(nil)
	if sl == nil {
		t.Errorf("SlogLogger(nil): got nil")
	}
}

func TestExitOnComplete_CallsExitFn(t *testing.T) {
	exitCalls := atomic.Int32{}
	exitedWith := atomic.Int32{}
	exitFn := func(code int) {
		exitCalls.Add(1)
		exitedWith.Store(int32(code))
	}

	m := New(
		WithLogger(NoopLogger()),
		WithBudget(1*time.Second),
		WithExitOnComplete(0, 1),
		withExitFn(exitFn),
	)
	_ = m.Register("h", func(_ context.Context) error { return nil })

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() {
		_ = m.Listen(ctx)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	p, _ := os.FindProcess(os.Getpid())
	_ = p.Signal(syscall.SIGTERM)

	<-done

	if exitCalls.Load() != 1 {
		t.Errorf("exitFn calls: got %d, want 1", exitCalls.Load())
	}
	if exitedWith.Load() != 0 {
		t.Errorf("exit code: got %d, want 0 (success)", exitedWith.Load())
	}
}

// errSentinelTest exercises the errSentinel string-backed error.
func TestErrSentinel_Error(t *testing.T) {
	e := errSentinel("test")
	if e.Error() != "test" {
		t.Errorf("errSentinel.Error: got %q", e.Error())
	}
}
