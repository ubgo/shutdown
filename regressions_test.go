// regressions_test.go — Regression tests pinned to MISSION.md §8.
//
// Each test in this file maps 1:1 to a shortcoming catalogued in
// `private-repo/boilerplate/ubgo/shutdown/MISSION.md §8`. The mapping is
// preserved in the test name (TestMission_8_XX_…) and the leading comment
// so future contributors can see the bug-class each test guards against.
//
// These are not new behavioural tests — the relevant behaviours are also
// covered by shutdown_test.go and listen_test.go. The point of this file
// is to make the §8 → code → test trail discoverable: when a future
// refactor breaks one, the test name tells you which property of the
// design was lost.
package shutdown

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// 8.1 — No phase ordering (handlers all run in one bucket, sequentially).
//
// Old behaviour: every handler ran in registration order inside one
// implicit bucket, so refactors silently changed shutdown order.
// New behaviour: handlers placed in different phases via WithPhase MUST
// run in ascending phase order regardless of registration order.
func TestMission_8_1_PhaseOrderingHonoured(t *testing.T) {
	m := New(WithLogger(NoopLogger()), WithBudget(2*time.Second))

	var (
		mu    sync.Mutex
		order []string
	)
	rec := func(name string) HandlerFunc {
		return func(_ context.Context) error {
			mu.Lock()
			order = append(order, name)
			mu.Unlock()
			return nil
		}
	}

	// Register intentionally out of phase order to prove ordering is by
	// phase, not registration sequence.
	_ = m.Register("flush-logs", rec("flush-logs"), WithPhase(PhaseFlushLogs))
	_ = m.Register("close-db", rec("close-db"), WithPhase(PhaseCloseClients))
	_ = m.Register("readiness", rec("readiness"), WithPhase(PhasePreShutdown))
	_ = m.Register("stop-http", rec("stop-http"), WithPhase(PhaseStopAccepting))

	if err := m.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	want := []string{"readiness", "stop-http", "close-db", "flush-logs"}
	if len(order) != len(want) {
		t.Fatalf("order len=%d, want %d (%v)", len(order), len(want), order)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("step %d: got %q want %q (full=%v)", i, order[i], want[i], order)
		}
	}
}

// 8.2 — No parallelism (independent closes wait on each other).
//
// Old behaviour: handlers ran sequentially even when independent.
// New behaviour: handlers in the same phase run in parallel by default,
// so total wall-clock time is closer to max(handler) than sum(handler).
func TestMission_8_2_ParallelWithinPhase(t *testing.T) {
	m := New(WithLogger(NoopLogger()), WithBudget(2*time.Second))

	const each = 100 * time.Millisecond
	mk := func() HandlerFunc {
		return func(_ context.Context) error {
			time.Sleep(each)
			return nil
		}
	}
	for _, n := range []string{"a", "b", "c", "d"} {
		_ = m.Register(n, mk(), WithPhase(PhaseCloseClients))
	}

	start := time.Now()
	_ = m.Shutdown(context.Background())
	elapsed := time.Since(start)

	// 4 sequential handlers would be 400ms+; parallel must be well under.
	if elapsed > 250*time.Millisecond {
		t.Errorf("parallel execution lost: 4×%v handlers took %v (want < 250ms)", each, elapsed)
	}
}

// 8.3 — Hardcoded zap dependency in core.
//
// Old behaviour: `lace/shutdown` imported zap directly so users on
// slog/zerolog inherited an unwanted dep.
// New behaviour: core defines its own 3-method Logger interface and ships
// a slog-backed default; zap integration lives in a separate contrib.
func TestMission_8_3_LoggerIsPluggable(t *testing.T) {
	type rec struct {
		level, msg string
	}
	got := []rec{}
	var mu sync.Mutex
	custom := loggerFunc{
		info: func(msg string, _ ...any) { mu.Lock(); got = append(got, rec{"info", msg}); mu.Unlock() },
		warn: func(msg string, _ ...any) { mu.Lock(); got = append(got, rec{"warn", msg}); mu.Unlock() },
		err:  func(msg string, _ ...any) { mu.Lock(); got = append(got, rec{"err", msg}); mu.Unlock() },
	}

	m := New(WithLogger(custom), WithBudget(time.Second))
	_ = m.Register("h", func(_ context.Context) error { return nil })
	_ = m.Shutdown(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if len(got) == 0 {
		t.Fatal("custom logger received no messages — Logger interface not honoured")
	}
}

type loggerFunc struct {
	info, warn, err func(string, ...any)
}

func (l loggerFunc) Info(msg string, kv ...any)  { l.info(msg, kv...) }
func (l loggerFunc) Warn(msg string, kv ...any)  { l.warn(msg, kv...) }
func (l loggerFunc) Error(msg string, kv ...any) { l.err(msg, kv...) }

// 8.4 — Hardcoded `time.Sleep` at the end is an anti-pattern.
//
// Old behaviour: `lace/shutdown` slept N seconds after handlers finished.
// New behaviour: shutdown returns as soon as the last phase completes;
// total duration tracks the longest critical path, not a fixed sleep.
func TestMission_8_4_NoMandatorySleep(t *testing.T) {
	m := New(WithLogger(NoopLogger()), WithBudget(5*time.Second))
	_ = m.Register("fast", func(_ context.Context) error { return nil })

	start := time.Now()
	_ = m.Shutdown(context.Background())
	elapsed := time.Since(start)

	// The old lib slept ~3s by default. We require the call to return promptly.
	if elapsed > 200*time.Millisecond {
		t.Errorf("Shutdown of trivial handler took %v — sleep regression?", elapsed)
	}
}

// 8.5 — `defer New().Listen()` antipattern in main.
//
// Old behaviour was a usage problem, not a library bug; nothing prevented
// the antipattern. New library still cannot prevent the antipattern at
// compile time, but the docs/examples uniformly use the explicit pattern
// (`mgr.Listen(ctx)` as the last statement of `main`). This test asserts
// the *property* the antipattern relied on — that Listen returns when ctx
// is cancelled — so the explicit pattern remains viable.
func TestMission_8_5_ListenReturnsOnCtxCancel(t *testing.T) {
	m := New(WithLogger(NoopLogger()), WithBudget(time.Second))

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	err := m.Listen(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Listen ctx cancel: got %v want context.Canceled", err)
	}
}

// 8.6 — No second-signal handling.
//
// Old behaviour: `Listen()` read the signal channel once; a second SIGINT
// during shutdown was silently dropped, so a hanging handler could only be
// killed with `kill -9`.
// New behaviour: a second signal during shutdown triggers force-exit.
func TestMission_8_6_SecondSignalForcesExit(t *testing.T) {
	exitCalls := atomic.Int32{}
	exitCode := atomic.Int32{}

	m := New(
		WithLogger(NoopLogger()),
		WithBudget(5*time.Second),
		withExitFn(func(code int) {
			exitCalls.Add(1)
			exitCode.Store(int32(code))
		}),
	)
	_ = m.Register("hang", func(ctx context.Context) error {
		select {
		case <-time.After(3 * time.Second):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}, WithTimeout(3*time.Second))

	done := make(chan error, 1)
	go func() { done <- m.Listen(context.Background()) }()
	time.Sleep(20 * time.Millisecond)

	p, _ := os.FindProcess(os.Getpid())
	_ = p.Signal(syscall.SIGTERM) // first → start shutdown
	time.Sleep(50 * time.Millisecond)
	_ = p.Signal(syscall.SIGTERM) // second → force exit

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Listen never returned after second signal")
	}
	if exitCalls.Load() == 0 {
		t.Error("force-exit did not fire on second signal")
	}
	if got := exitCode.Load(); got != 130 {
		t.Errorf("force-exit code = %d, want 130", got)
	}
}

// 8.7 — No watchdog (a hanging handler stalls forever).
//
// Old behaviour: a handler that ignored ctx held shutdown forever.
// New behaviour: budget+grace timer hard-exits via the watchdog goroutine.
func TestMission_8_7_WatchdogForceExitsOnBudgetOverrun(t *testing.T) {
	exitCalls := atomic.Int32{}
	m := New(
		WithLogger(NoopLogger()),
		WithBudget(50*time.Millisecond),
		WithWatchdogGrace(20*time.Millisecond),
		withExitFn(func(_ int) { exitCalls.Add(1) }),
	)
	_ = m.Register("hang", func(_ context.Context) error {
		time.Sleep(300 * time.Millisecond) // ignores ctx — the bug class
		return nil
	}, WithTimeout(500*time.Millisecond))

	_ = m.Shutdown(context.Background())
	if exitCalls.Load() == 0 {
		t.Error("watchdog did not force-exit on budget overrun")
	}
}

// 8.8 — No programmatic trigger.
//
// Old behaviour: shutdown could only be initiated by an OS signal.
// New behaviour: Manager.Shutdown(ctx) runs the same execution path as a
// signal-driven shutdown — usable from tests, panic recovery, or an
// `/admin/shutdown` HTTP endpoint.
func TestMission_8_8_ProgrammaticTrigger(t *testing.T) {
	m := New(WithLogger(NoopLogger()), WithBudget(time.Second))
	ran := atomic.Int32{}
	_ = m.Register("h", func(_ context.Context) error {
		ran.Add(1)
		return nil
	})

	if err := m.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if ran.Load() != 1 {
		t.Errorf("handler ran %d times, want 1", ran.Load())
	}
}

// 8.9 — Errors stop at the first failure (no aggregation).
//
// Old behaviour: `runHandlers` returned on first error; remaining handlers
// (Redis close, OTEL flush) were skipped, leaking resources and losing
// telemetry.
// New behaviour: errors.Join aggregation by default (ContinueOnError).
func TestMission_8_9_ErrorAggregation(t *testing.T) {
	m := New(WithLogger(NoopLogger()), WithBudget(time.Second))

	_ = m.Register("first", func(_ context.Context) error { return errors.New("first-err") },
		WithPhase(PhaseCloseClients))
	_ = m.Register("second", func(_ context.Context) error { return errors.New("second-err") },
		WithPhase(PhaseCloseClients))
	_ = m.Register("third-different-phase", func(_ context.Context) error { return errors.New("third-err") },
		WithPhase(PhaseFlushLogs))

	err := m.Shutdown(context.Background())
	if err == nil {
		t.Fatal("expected aggregated error")
	}
	for _, fragment := range []string{"first-err", "second-err", "third-err"} {
		if !contains(err.Error(), fragment) {
			t.Errorf("aggregate missing %q: %v", fragment, err)
		}
	}
}

// 8.10 — Variadic registration without ordering.
//
// Old behaviour: `WithShutdownHandler` accepted handlers variadically with
// shared (i.e. zero) priority — no way to order within a registration.
// New behaviour: each Register call carries its own name + phase + timeout.
// Re-registering the same name fails with ErrAlreadyRegistered.
func TestMission_8_10_PerHandlerOptions(t *testing.T) {
	m := New(WithLogger(NoopLogger()), WithBudget(time.Second))
	if err := m.Register("a", func(_ context.Context) error { return nil },
		WithPhase(PhaseStopAccepting), WithTimeout(50*time.Millisecond)); err != nil {
		t.Fatalf("Register a: %v", err)
	}
	if err := m.Register("b", func(_ context.Context) error { return nil },
		WithPhase(PhaseFlushLogs), WithTimeout(2*time.Second)); err != nil {
		t.Fatalf("Register b: %v", err)
	}
	if err := m.Register("a", func(_ context.Context) error { return nil }); !errors.Is(err, ErrAlreadyRegistered) {
		t.Errorf("re-register: got %v want ErrAlreadyRegistered", err)
	}
}

// 8.11 — No telemetry hooks (shutdown invisible to OTEL / Prometheus).
//
// Old behaviour: only four log lines were emitted; spans/metrics were
// impossible to wire without forking.
// New behaviour: Observer pattern fires OnSignal / OnPhaseStart /
// OnPhaseEnd / OnHandlerStart / OnHandlerEnd / OnComplete on every event.
// (shutdown-otel and shutdown-prom contribs use exactly this surface.)
func TestMission_8_11_ObserverPatternFiresAllCallbacks(t *testing.T) {
	m := New(WithLogger(NoopLogger()), WithBudget(time.Second))

	var (
		mu                                    sync.Mutex
		phaseStarts, phaseEnds                int
		handlerStarts, handlerEnds, completes int
	)
	m.Subscribe(Observer{
		OnPhaseStart:   func(_ Phase, _ int) { mu.Lock(); phaseStarts++; mu.Unlock() },
		OnPhaseEnd:     func(_ Phase, _ time.Duration, _ []error) { mu.Lock(); phaseEnds++; mu.Unlock() },
		OnHandlerStart: func(_ string, _ Phase) { mu.Lock(); handlerStarts++; mu.Unlock() },
		OnHandlerEnd:   func(_ string, _ Phase, _ time.Duration, _ error) { mu.Lock(); handlerEnds++; mu.Unlock() },
		OnComplete:     func(_ time.Duration, _ error) { mu.Lock(); completes++; mu.Unlock() },
	})

	_ = m.Register("a", func(_ context.Context) error { return nil }, WithPhase(PhaseStopAccepting))
	_ = m.Register("b", func(_ context.Context) error { return nil }, WithPhase(PhaseCloseClients))
	_ = m.Shutdown(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if phaseStarts != 2 || phaseEnds != 2 {
		t.Errorf("phase callbacks: starts=%d ends=%d, want 2/2", phaseStarts, phaseEnds)
	}
	if handlerStarts != 2 || handlerEnds != 2 {
		t.Errorf("handler callbacks: starts=%d ends=%d, want 2/2", handlerStarts, handlerEnds)
	}
	if completes != 1 {
		t.Errorf("OnComplete fired %d times, want 1", completes)
	}
}

// 8.12 — No integration with `ubgo/health` (drain flag).
//
// The integration itself lives in `contrib/shutdown-health`, which depends
// on `ubgo/health` being API-stable. The library *property* this depends
// on is that PhasePreShutdown runs before any listener-close phase; this
// test pins that property so the contrib's eventual implementation cannot
// silently regress.
func TestMission_8_12_PreShutdownRunsBeforeStopAccepting(t *testing.T) {
	m := New(WithLogger(NoopLogger()), WithBudget(time.Second))

	var phaseSeq []Phase
	var mu sync.Mutex
	rec := func(p Phase) HandlerFunc {
		return func(_ context.Context) error {
			mu.Lock()
			phaseSeq = append(phaseSeq, p)
			mu.Unlock()
			return nil
		}
	}

	_ = m.Register("listener-close", rec(PhaseStopAccepting), WithPhase(PhaseStopAccepting))
	_ = m.Register("readiness-flip", rec(PhasePreShutdown), WithPhase(PhasePreShutdown))

	_ = m.Shutdown(context.Background())

	if len(phaseSeq) != 2 {
		t.Fatalf("got %d phases, want 2 (%v)", len(phaseSeq), phaseSeq)
	}
	if phaseSeq[0] != PhasePreShutdown {
		t.Errorf("PhasePreShutdown did not run first: got %v", phaseSeq)
	}
}

// 8.13 — No exit-code control.
//
// Old behaviour: exit code was whatever Go's runtime set after main
// returned; orchestrators couldn't distinguish failed shutdown.
// New behaviour: WithExitOnComplete(success, failure) opts into explicit
// os.Exit with the right code for the run.
func TestMission_8_13_ExitCodeControl(t *testing.T) {
	exitCalls := atomic.Int32{}
	exitedWith := atomic.Int32{}

	t.Run("success", func(t *testing.T) {
		exitCalls.Store(0)
		exitedWith.Store(0)
		m := New(
			WithLogger(NoopLogger()),
			WithExitOnComplete(0, 1),
			withExitFn(func(c int) {
				exitCalls.Add(1)
				exitedWith.Store(int32(c))
			}),
		)
		_ = m.Register("ok", func(_ context.Context) error { return nil })

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		done := make(chan struct{})
		go func() { _ = m.Listen(ctx); close(done) }()
		time.Sleep(20 * time.Millisecond)
		p, _ := os.FindProcess(os.Getpid())
		_ = p.Signal(syscall.SIGTERM)
		<-done

		if exitCalls.Load() != 1 || exitedWith.Load() != 0 {
			t.Errorf("success exit: calls=%d code=%d, want 1/0", exitCalls.Load(), exitedWith.Load())
		}
	})

	t.Run("failure", func(t *testing.T) {
		exitCalls.Store(0)
		exitedWith.Store(0)
		m := New(
			WithLogger(NoopLogger()),
			WithExitOnComplete(0, 1),
			withExitFn(func(c int) {
				exitCalls.Add(1)
				exitedWith.Store(int32(c))
			}),
		)
		_ = m.Register("fail", func(_ context.Context) error { return errors.New("boom") })

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		done := make(chan struct{})
		go func() { _ = m.Listen(ctx); close(done) }()
		time.Sleep(20 * time.Millisecond)
		p, _ := os.FindProcess(os.Getpid())
		_ = p.Signal(syscall.SIGTERM)
		<-done

		if exitCalls.Load() != 1 || exitedWith.Load() != 1 {
			t.Errorf("failure exit: calls=%d code=%d, want 1/1", exitCalls.Load(), exitedWith.Load())
		}
	})
}
