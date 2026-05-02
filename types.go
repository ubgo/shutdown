package shutdown

import (
	"context"
	"fmt"
	"os"
	"time"
)

// HandlerFunc is the unit of work registered with a Manager. It receives
// a context bounded by the per-handler timeout (or the remaining global
// budget, whichever is shorter) and should return promptly when ctx is done.
type HandlerFunc func(ctx context.Context) error

// InterruptFunc is the cancellation half of an actor registration. It is
// called when the manager wants the actor to stop. Implementations should
// be quick and idempotent.
type InterruptFunc func(err error)

// RunFunc is the long-running half of an actor registration. It returns
// when the actor naturally exits (or its InterruptFunc was called).
type RunFunc func() error

// Phase is the ordering key for handler execution. Lower phases run first.
// Predefined constants cover the common k8s preStop drain pattern; a raw
// int is also valid for power users who want finer-grained sequencing.
type Phase int

// Predefined phases — match the typical k8s graceful-shutdown flow:
//
//  1. PhasePreShutdown      — flip drain flag (load balancer stops sending).
//  2. PhaseStopAccepting    — close listeners (no new requests accepted).
//  3. PhaseDrainTraffic     — wait for in-flight work to finish.
//  4. PhaseFlushQueues      — flush async producers and worker queues.
//  5. PhaseCloseClients     — close DB, cache, messaging clients.
//  6. PhaseFlushLogs        — flush logs and traces last so prior phase
//     errors reach the collector.
//  7. PhasePostShutdown     — final cleanup, exit-code reporting.
const (
	PhasePreShutdown   Phase = -100
	PhaseStopAccepting Phase = 0
	PhaseDrainTraffic  Phase = 100
	PhaseFlushQueues   Phase = 200
	PhaseCloseClients  Phase = 300
	PhaseFlushLogs     Phase = 400
	PhasePostShutdown  Phase = 500
)

// String returns the canonical phase name when one of the predefined
// constants matches; otherwise returns "phase=<n>".
func (p Phase) String() string {
	switch p {
	case PhasePreShutdown:
		return "PreShutdown"
	case PhaseStopAccepting:
		return "StopAccepting"
	case PhaseDrainTraffic:
		return "DrainTraffic"
	case PhaseFlushQueues:
		return "FlushQueues"
	case PhaseCloseClients:
		return "CloseClients"
	case PhaseFlushLogs:
		return "FlushLogs"
	case PhasePostShutdown:
		return "PostShutdown"
	default:
		return fmt.Sprintf("phase=%d", int(p))
	}
}

// ErrorPolicy decides what happens when a handler returns an error.
type ErrorPolicy int

const (
	// ContinueOnError keeps running remaining handlers in the same phase
	// and proceeds to subsequent phases. All errors are aggregated via
	// errors.Join and returned at the end. Default.
	ContinueOnError ErrorPolicy = iota
	// StopOnError aborts the phase on the first failure and returns
	// immediately, skipping subsequent phases.
	StopOnError
)

// Logger is the minimal logging contract. Adapters ship as separate modules:
// shutdown-zap, shutdown-slog. The default Manager uses log/slog.
type Logger interface {
	Info(msg string, fields ...any)
	Warn(msg string, fields ...any)
	Error(msg string, fields ...any)
}

// Observer fan-out for adapters. All callbacks are optional and may be nil.
// Observers fire synchronously; long-running observers should fan out to a
// goroutine themselves.
type Observer struct {
	OnSignal       func(sig os.Signal)
	OnPhaseStart   func(phase Phase, handlerCount int)
	OnPhaseEnd     func(phase Phase, dur time.Duration, errs []error)
	OnHandlerStart func(name string, phase Phase)
	OnHandlerEnd   func(name string, phase Phase, dur time.Duration, err error)
	OnComplete     func(totalDur time.Duration, err error)
}
