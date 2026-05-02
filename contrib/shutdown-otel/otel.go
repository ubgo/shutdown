// Package shutdownotel provides a shutdown.Observer that emits OpenTelemetry
// spans for each phase and handler.
//
// One root span "shutdown" is opened on the first OnPhaseStart and closed on
// OnComplete. Each phase opens a child "shutdown.phase.<name>" span; each
// handler opens a leaf "shutdown.handler.<name>" span. Errors are recorded
// on the leaf span via span.RecordError + span.SetStatus.
//
// The observer is safe to subscribe before Shutdown begins; the root span is
// lazily started when the first phase begins.
package shutdownotel

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/ubgo/shutdown"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Observer returns an observer that emits OTEL spans through the provided
// tracer. A nil tracer is replaced with a no-op tracer.
func Observer(tracer trace.Tracer) shutdown.Observer {
	if tracer == nil {
		tracer = trace.NewNoopTracerProvider().Tracer("shutdown-otel")
	}

	state := &spanState{
		tracer:        tracer,
		phaseSpans:    map[shutdown.Phase]trace.Span{},
		phaseCtx:      map[shutdown.Phase]context.Context{},
		handlerSpans: map[handlerKey]trace.Span{},
	}

	return shutdown.Observer{
		OnSignal: func(sig os.Signal) {
			state.ensureRoot()
			state.rootSpan.AddEvent("signal", trace.WithAttributes(
				attribute.String("signal", sig.String()),
			))
		},
		OnPhaseStart: func(p shutdown.Phase, n int) {
			state.startPhase(p, n)
		},
		OnPhaseEnd: func(p shutdown.Phase, dur time.Duration, errs []error) {
			state.endPhase(p, dur, len(errs))
		},
		OnHandlerStart: func(name string, p shutdown.Phase) {
			state.startHandler(name, p)
		},
		OnHandlerEnd: func(name string, p shutdown.Phase, dur time.Duration, err error) {
			state.endHandler(name, p, dur, err)
		},
		OnComplete: func(total time.Duration, err error) {
			state.complete(total, err)
		},
	}
}

type handlerKey struct {
	phase shutdown.Phase
	name  string
}

type spanState struct {
	mu       sync.Mutex
	tracer   trace.Tracer
	rootCtx  context.Context
	rootSpan trace.Span

	phaseSpans   map[shutdown.Phase]trace.Span
	phaseCtx     map[shutdown.Phase]context.Context
	handlerSpans map[handlerKey]trace.Span
}

func (s *spanState) ensureRoot() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rootSpan != nil {
		return
	}
	s.rootCtx, s.rootSpan = s.tracer.Start(context.Background(), "shutdown")
}

func (s *spanState) startPhase(p shutdown.Phase, handlerCount int) {
	s.ensureRoot()
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, span := s.tracer.Start(s.rootCtx, "shutdown.phase."+p.String(),
		trace.WithAttributes(
			attribute.String("phase", p.String()),
			attribute.Int("handlers", handlerCount),
		),
	)
	s.phaseSpans[p] = span
	s.phaseCtx[p] = ctx
}

func (s *spanState) endPhase(p shutdown.Phase, dur time.Duration, errCount int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	span, ok := s.phaseSpans[p]
	if !ok {
		return
	}
	span.SetAttributes(
		attribute.Int64("duration_ms", dur.Milliseconds()),
		attribute.Int("errors", errCount),
	)
	if errCount > 0 {
		span.SetStatus(codes.Error, "phase had errors")
	}
	span.End()
	delete(s.phaseSpans, p)
	delete(s.phaseCtx, p)
}

func (s *spanState) startHandler(name string, p shutdown.Phase) {
	s.mu.Lock()
	parent, ok := s.phaseCtx[p]
	if !ok {
		parent = s.rootCtx
	}
	_, span := s.tracer.Start(parent, "shutdown.handler."+name,
		trace.WithAttributes(
			attribute.String("name", name),
			attribute.String("phase", p.String()),
		),
	)
	s.handlerSpans[handlerKey{p, name}] = span
	s.mu.Unlock()
}

func (s *spanState) endHandler(name string, p shutdown.Phase, dur time.Duration, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	span, ok := s.handlerSpans[handlerKey{p, name}]
	if !ok {
		return
	}
	span.SetAttributes(attribute.Int64("duration_ms", dur.Milliseconds()))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
	delete(s.handlerSpans, handlerKey{p, name})
}

func (s *spanState) complete(total time.Duration, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rootSpan == nil {
		return
	}
	s.rootSpan.SetAttributes(attribute.Int64("total_ms", total.Milliseconds()))
	if err != nil {
		s.rootSpan.RecordError(err)
		s.rootSpan.SetStatus(codes.Error, err.Error())
	} else {
		s.rootSpan.SetStatus(codes.Ok, "")
	}
	s.rootSpan.End()
	s.rootSpan = nil
	s.rootCtx = nil
}
