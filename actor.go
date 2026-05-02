package shutdown

import (
	"context"
	"errors"
	"sync"
	"time"
)

// actorRegistration is the internal record for one registered actor.
//
// Actors are long-running goroutines. The Run function is called by the
// caller (e.g. `go actor.Start()` in main); the manager holds the Interrupt
// function and calls it during the configured phase. Inspired by oklog/run.
type actorRegistration struct {
	name      string
	interrupt InterruptFunc
	phase     Phase
	timeout   time.Duration

	// completed is closed when the actor's Run returned naturally (the
	// caller invoked actor.Done(err) after the actor goroutine exited).
	// The interrupt handler waits on this so a phase doesn't proceed
	// until the actor genuinely stopped.
	completed     chan struct{}
	completedOnce sync.Once
	completionErr error
}

// asHandler wraps the actor's interrupt+completion handshake into a
// HandlerFunc the manager can place in its phase buckets.
//
// The two-step handshake (call interrupt, then wait on `completed`) is
// what distinguishes an actor from a regular handler: regular handlers
// own their cleanup synchronously, but an actor's run loop is in another
// goroutine that the manager does not control. We must signal it AND
// wait for it — otherwise the next phase could start while the actor is
// still mid-cleanup.
func (a *actorRegistration) asHandler() HandlerFunc {
	return func(ctx context.Context) error {
		if a.interrupt != nil {
			a.interrupt(errors.New("shutdown: requested"))
		}
		select {
		case <-a.completed:
			return a.completionErr
		case <-ctx.Done():
			// The actor is still running, but we've blown the per-actor
			// timeout (or the manager budget). Returning ctx.Err() lets
			// the runner aggregate this as a failure; the actor goroutine
			// continues to run in the background and may still call
			// handle.Done eventually — that call is now a no-op (the
			// channel is closed only the first time).
			return ctx.Err()
		}
	}
}

// RegisterActor registers a long-running actor (goroutine-style service)
// with the Manager. The actor is signalled to stop via interrupt during
// its phase, and the manager waits up to the per-actor timeout (or the
// remaining global budget, whichever is shorter) for the actor to confirm
// completion via the returned ActorHandle.
//
// Typical use:
//
//	handle, err := mgr.RegisterActor("worker", workerStop,
//	    shutdown.WithActorPhase(shutdown.PhaseDrainTraffic))
//	go func() {
//	    err := workerLoop()       // blocks until workerStop is called
//	    handle.Done(err)          // signals actor exited
//	}()
//
// Note: the run loop itself is not held by the manager. The caller is
// responsible for spawning the goroutine that runs the work; the manager
// only owns the interrupt + completion handshake.
func (m *Manager) RegisterActor(name string, interrupt InterruptFunc, opts ...ActorOption) (*ActorHandle, error) {
	if name == "" {
		return nil, ErrEmptyName
	}
	if interrupt == nil {
		return nil, errNilInterrupt
	}

	a := &actorRegistration{
		name:      name,
		interrupt: interrupt,
		phase:     PhaseDrainTraffic,
		timeout:   30 * time.Second,
		completed: make(chan struct{}),
	}
	for _, o := range opts {
		o(a)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, ErrClosed
	}
	for _, existing := range m.actors {
		if existing.name == name {
			return nil, ErrAlreadyRegistered
		}
	}
	m.actors = append(m.actors, a)
	return &ActorHandle{actor: a}, nil
}

// ActorHandle is returned from RegisterActor. Call Done(err) when the
// actor's run loop has exited so the manager can proceed to the next phase.
type ActorHandle struct {
	actor *actorRegistration
}

// Done signals that the actor's run loop has returned. err is the run
// loop's exit error (nil if it returned cleanly). Idempotent — only the
// first call has effect.
func (h *ActorHandle) Done(err error) {
	if h == nil || h.actor == nil {
		return
	}
	h.actor.completedOnce.Do(func() {
		h.actor.completionErr = err
		close(h.actor.completed)
	})
}

// ActorOption configures a RegisterActor call.
type ActorOption func(*actorRegistration)

// WithActorPhase places the actor's interrupt step in a specific phase.
// Default: PhaseDrainTraffic.
func WithActorPhase(p Phase) ActorOption {
	return func(a *actorRegistration) { a.phase = p }
}

// WithActorTimeout caps how long the manager waits for the actor's run
// loop to confirm completion via Done after interrupt is called. Default: 30s.
func WithActorTimeout(d time.Duration) ActorOption {
	return func(a *actorRegistration) { a.timeout = d }
}

var errNilInterrupt = errSentinel("shutdown: actor interrupt function must not be nil")
