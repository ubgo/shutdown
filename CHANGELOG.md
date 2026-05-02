# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [v0.2.0] - 2026-05-02

### Fixed

- **Panicking handlers now surface as errors.** Previously a `panic()` inside
  a handler was caught by the deferred `recover()` and silently turned into
  a `nil` return — the aggregated error said the shutdown succeeded. The
  recover now returns a `*PanicError{Name, Value}` so the panic appears in
  `errors.Join`'s aggregate, in the log line, and in the observer's
  `OnHandlerEnd`. (`register.go`)
- **`Phase.String()` for custom phases now reports the value.** Phase(42)
  returned the literal string `"phase=custom"`; it now returns `"phase=42"`.
- **`OnSignal` hooks for unlisted signals are auto-included.** Registering a
  hook for SIGUSR1 without also passing `WithSignals(syscall.SIGUSR1, ...)`
  silently never fired because `signal.Notify` wasn't watching for it. The
  hooked signal set is now merged with the listened set when `Listen` starts.
- Removed dead `context` import + `var _ = context.Background` in `logger.go`.
- Corrected `OnSignal` godoc — referenced a non-existent `ContinueListening`
  symbol.

### Added

- `WithHandlerDefaultTimeout(d time.Duration)` — sets the default per-handler
  timeout (formerly hard-coded to 5s) used when `Register` does not pass
  `WithTimeout`.
- `PanicError` exported type so callers can `errors.As` on it from the
  aggregated error.

### Removed

- **`WithRequired()` register option** — the documented behaviour did not
  match the implementation (both branches aggregated errors identically) and
  the option had no real effect at runtime. Drop now while pre-v1, free up
  the slot for a properly-designed required/optional split later.

## [v0.1.0] - 2026-05-01

### Added

- Initial implementation of the `shutdown` core module.
- Phase-based ordering via `Phase` enum (PreShutdown / StopAccepting / DrainTraffic / FlushQueues / CloseClients / FlushLogs / PostShutdown). Free-form `int` phases also accepted.
- Parallel handler execution within a phase by default; opt out via `WithSerial(phase)`.
- Per-handler `WithTimeout` and `WithPhase` options.
- Programmatic `Shutdown(ctx)` and signal-driven `Listen(ctx)` — same execution path.
- Force-exit on second signal (`WithForceOnSecondSignal(true, 130)`, default).
- Watchdog: hard-exits after `WithBudget` plus `WithWatchdogGrace` if handlers hang.
- Error aggregation via `errors.Join`; `WithErrorPolicy` for ContinueOnError (default) or StopOnError.
- `WithExitOnComplete(success, failure int)` opts into explicit `os.Exit` at end.
- Observer pattern for adapter integration: `OnSignal`, `OnPhaseStart`, `OnPhaseEnd`, `OnHandlerStart`, `OnHandlerEnd`, `OnComplete`.
- Pluggable `Logger` interface; default backed by `log/slog`. `NoopLogger` and `SlogLogger(*slog.Logger)` helpers.
- `RegisterActor(name, interrupt, opts...)` for long-running goroutines (oklog/run-style run+interrupt pairs); returns `*ActorHandle` with `Done(err error)`.
- `OnSignal(sig, fn)` for non-shutdown signal hooks (SIGHUP for reload, SIGUSR1 for log rotation).
- Taskfile, CI workflows, README, NOTICE.
- Licensed under Apache License 2.0.

[Unreleased]: https://github.com/ubgo/shutdown/compare/v0.2.0...HEAD
[v0.2.0]: https://github.com/ubgo/shutdown/compare/v0.1.0...v0.2.0
[v0.1.0]: https://github.com/ubgo/shutdown/releases/tag/v0.1.0
