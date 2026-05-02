// Package shutdownzap provides a shutdown.Observer that emits structured
// log lines via Uber's zap logger.
//
// Subscribe the returned observer to your manager:
//
//	logger, _ := zap.NewProduction()
//	mgr.Subscribe(shutdownzap.Observer(logger))
//
// This is independent of the shutdown.Manager's internal Logger (which
// uses log/slog by default) — observers see every shutdown event with
// timing, while the internal logger emits a smaller fixed message set.
package shutdownzap

import (
	"errors"
	"os"
	"time"

	"github.com/ubgo/shutdown"
	"go.uber.org/zap"
)

// Observer returns an observer that logs every callback at INFO level.
// Errors are logged as field values, never as zap.Error to keep the
// "shutdown: ..." prefix consistent across observers.
func Observer(l *zap.Logger) shutdown.Observer {
	if l == nil {
		l = zap.NewNop()
	}
	return shutdown.Observer{
		OnSignal: func(sig os.Signal) {
			l.Info("shutdown: signal received", zap.String("signal", sig.String()))
		},
		OnPhaseStart: func(p shutdown.Phase, n int) {
			l.Info("shutdown: phase start",
				zap.String("phase", p.String()),
				zap.Int("handlers", n))
		},
		OnPhaseEnd: func(p shutdown.Phase, dur time.Duration, errs []error) {
			fields := []zap.Field{
				zap.String("phase", p.String()),
				zap.Duration("duration", dur),
				zap.Int("errors", len(errs)),
			}
			if len(errs) > 0 {
				fields = append(fields, zap.Error(errors.Join(errs...)))
			}
			l.Info("shutdown: phase end", fields...)
		},
		OnHandlerStart: func(name string, p shutdown.Phase) {
			l.Info("shutdown: handler start",
				zap.String("name", name),
				zap.String("phase", p.String()))
		},
		OnHandlerEnd: func(name string, p shutdown.Phase, dur time.Duration, err error) {
			fields := []zap.Field{
				zap.String("name", name),
				zap.String("phase", p.String()),
				zap.Duration("duration", dur),
			}
			if err != nil {
				fields = append(fields, zap.Error(err))
				l.Error("shutdown: handler failed", fields...)
				return
			}
			l.Info("shutdown: handler end", fields...)
		},
		OnComplete: func(total time.Duration, err error) {
			fields := []zap.Field{zap.Duration("total", total)}
			if err != nil {
				fields = append(fields, zap.Error(err))
				l.Error("shutdown: completed with errors", fields...)
				return
			}
			l.Info("shutdown: completed", fields...)
		},
	}
}
