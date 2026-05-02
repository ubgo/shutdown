# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Initial implementation of the `shutdown` core module.
- Phase-based ordering via `Phase` enum (PreShutdown / StopAccepting / DrainTraffic / FlushQueues / CloseClients / FlushLogs / PostShutdown). Free-form `int` phases also accepted.
- Parallel handler execution within a phase by default; opt out via `WithSerial(phase)`.
- Per-handler `WithTimeout` and `WithRequired` options; `WithPhase` to place handler in a specific phase.
- Programmatic `Shutdown(ctx)` and signal-driven `Listen(ctx)` — same execution path.
- Force-exit on second signal (`WithForceOnSecondSignal(true, 130)`, default).
- Watchdog: hard-exits after `WithBudget` plus `WithWatchdogGrace` if handlers hang.
- Error aggregation via `errors.Join`; `WithErrorPolicy` for ContinueOnError (default) or StopOnError.
- `WithExitOnComplete(success, failure int)` opts into explicit `os.Exit` at end.
- Observer pattern for adapter integration: `OnSignal`, `OnPhaseStart`, `OnPhaseEnd`, `OnHandlerStart`, `OnHandlerEnd`, `OnComplete`.
- Pluggable `Logger` interface; default backed by `log/slog`. `NoopLogger` and `SlogLogger(*slog.Logger)` helpers.
- `RegisterActor(name, interrupt, opts...)` for long-running goroutines (oklog/run-style run+interrupt pairs); returns `*ActorHandle` with `Done(err error)`.
- `OnSignal(sig, fn)` for non-shutdown signal hooks (SIGHUP for reload, SIGUSR1 for log rotation).
- 89.4% statement coverage with race detector enforced; tests exercise phase ordering, parallel execution, per-handler timeout, error aggregation, idempotent Shutdown, force-exit, watchdog, observer fan-out.
- Taskfile, CI workflows, README, NOTICE.
- Licensed under Apache License 2.0.

[Unreleased]: https://github.com/ubgo/shutdown/compare/v0.0.0...HEAD
