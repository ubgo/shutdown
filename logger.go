package shutdown

import (
	"log/slog"
)

// noopLogger discards everything. Used when the Manager is constructed
// without a logger override and no slog default is desired.
type noopLogger struct{}

func (noopLogger) Info(string, ...any)  {}
func (noopLogger) Warn(string, ...any)  {}
func (noopLogger) Error(string, ...any) {}

// slogLogger is the default Logger implementation, backed by log/slog.
//
// It accepts a *slog.Logger so callers can provide their own handler
// (JSON, text, custom). When constructed via defaultLogger() it uses
// slog.Default() so log lines flow through whatever handler the caller
// has installed globally.
type slogLogger struct {
	l *slog.Logger
}

func newSlogLogger(l *slog.Logger) Logger {
	if l == nil {
		l = slog.Default()
	}
	return &slogLogger{l: l}
}

func (s *slogLogger) Info(msg string, fields ...any)  { s.l.Info(msg, fields...) }
func (s *slogLogger) Warn(msg string, fields ...any)  { s.l.Warn(msg, fields...) }
func (s *slogLogger) Error(msg string, fields ...any) { s.l.Error(msg, fields...) }

// defaultLogger returns the Logger used when WithLogger is not supplied.
func defaultLogger() Logger {
	return newSlogLogger(nil)
}

// SlogLogger wraps a *slog.Logger as a shutdown.Logger. Useful when the
// caller wants to pass a specific *slog.Logger rather than relying on
// slog.Default().
func SlogLogger(l *slog.Logger) Logger {
	return newSlogLogger(l)
}

// NoopLogger returns a Logger that discards all messages. Useful in tests
// or when the application logs shutdown events through its observer hooks
// instead of the Logger interface.
func NoopLogger() Logger {
	return noopLogger{}
}
