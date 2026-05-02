# shutdown

Phased, parallel-within-phase, observable graceful shutdown manager for long-running Go services. Designed for Kubernetes, systemd, and any orchestrator that delivers SIGTERM with a grace period.

Zero third-party dependencies in the core. Adapter modules for Zap / slog / OpenTelemetry / Prometheus / `ubgo/health` / Gin / Chi / Echo / Fiber / `uber-go/fx` ship under `contrib/`.

## Install

```sh
go get github.com/ubgo/shutdown
```

## Quick start

```go
package main

import (
    "context"
    "log"
    "time"

    "github.com/ubgo/shutdown"
)

func main() {
    mgr := shutdown.New(
        shutdown.WithBudget(30 * time.Second),
    )

    mgr.Register("http",  httpServer.Shutdown,
        shutdown.WithPhase(shutdown.PhaseStopAccepting))
    mgr.Register("nats",  natsConn.Drain,
        shutdown.WithPhase(shutdown.PhaseDrainTraffic))
    mgr.Register("db",    db.Close,
        shutdown.WithPhase(shutdown.PhaseCloseClients))
    mgr.Register("redis", redisClient.Close,
        shutdown.WithPhase(shutdown.PhaseCloseClients)) // parallel with db
    mgr.Register("otel",  otelProvider.Shutdown,
        shutdown.WithPhase(shutdown.PhaseFlushLogs))

    if err := mgr.Listen(context.Background()); err != nil {
        log.Fatal(err)
    }
}
```

`Listen` blocks until SIGTERM/SIGINT, then runs every registered handler in phase order. Handlers in the same phase run in parallel. The whole thing is bounded by `WithBudget`; a watchdog hard-exits if budget plus grace expires.

## Phases

The seven predefined phases match the typical k8s preStop drain pattern. Lower phases run first.

| Phase | Value | Typical handlers |
|-------|-------|------------------|
| `PhasePreShutdown` | -100 | flip drain flag (load balancer stops sending) |
| `PhaseStopAccepting` | 0 | close HTTP listeners |
| `PhaseDrainTraffic` | 100 | wait for in-flight requests; NATS drain |
| `PhaseFlushQueues` | 200 | flush async producers, drain workers |
| `PhaseCloseClients` | 300 | close DB / cache / messaging clients |
| `PhaseFlushLogs` | 400 | flush logs and traces last |
| `PhasePostShutdown` | 500 | final cleanup |

Phases are plain `int` — pass any value to `WithPhase` if you need finer-grained ordering.

## Force-exit on second signal

By default, a second SIGTERM/SIGINT during shutdown calls `os.Exit(130)` immediately — the operator's escape hatch when a handler hangs. Disable via `WithForceOnSecondSignal(false, 0)`.

## Watchdog

`WithBudget(d)` sets the total wall-clock budget. After the budget plus a 1-second grace period (configurable via `WithWatchdogGrace`), the watchdog calls `os.Exit(failureCode)` with the names of the stuck handlers logged. No more zombie processes.

## Programmatic trigger

Tests, panic-recovery middleware, custom HTTP `/admin/shutdown` endpoints, and `health` failure paths can all trigger shutdown without an OS signal:

```go
err := mgr.Shutdown(ctx)
```

Same execution path as `Listen`.

## Reload signals (SIGHUP)

```go
mgr.OnSignal(syscall.SIGHUP, func(ctx context.Context, _ os.Signal) {
    config.Reload()
})
```

The hook fires; shutdown is NOT triggered. Useful for log rotation (`SIGUSR1`), config reload (`SIGHUP`), and similar gunicorn-style signal patterns.

## Actor pattern (oklog/run-style)

For long-running goroutines (workers, schedulers) where the run loop and the cancel mechanism are distinct:

```go
handle, _ := mgr.RegisterActor("worker", workerStop,
    shutdown.WithActorPhase(shutdown.PhaseDrainTraffic))

go func() {
    err := workerLoop()  // blocks until workerStop is called
    handle.Done(err)     // signals actor completed
}()
```

The manager calls `workerStop` during the configured phase, then waits up to the per-actor timeout for `handle.Done` to be called.

## Observer pattern

Adapters subscribe to lifecycle callbacks instead of polluting the core:

```go
mgr.Subscribe(shutdown.Observer{
    OnPhaseStart: func(p shutdown.Phase, n int) { /* OTEL span start */ },
    OnPhaseEnd:   func(p shutdown.Phase, dur time.Duration, errs []error) { /* span end */ },
    OnHandlerEnd: func(name string, p shutdown.Phase, dur time.Duration, err error) { /* prom metric */ },
    OnComplete:   func(total time.Duration, err error) { /* alert webhook */ },
})
```

`shutdown-otel`, `shutdown-prom`, and `shutdown-health` contribs use this pattern.

## Adapters

Adapter modules ship as separate Go modules under `contrib/`. Import only the ones you use; each pulls only its own dependencies.

| Adapter | Module path | Role |
|---------|-------------|------|
| `shutdown-zap` | `github.com/ubgo/shutdown/contrib/shutdown-zap` | Zap `Logger` adapter |
| `shutdown-slog` | `github.com/ubgo/shutdown/contrib/shutdown-slog` | Explicit `*slog.Logger` adapter |
| `shutdown-otel` | `github.com/ubgo/shutdown/contrib/shutdown-otel` | OpenTelemetry spans per phase + handler |
| `shutdown-prom` | `github.com/ubgo/shutdown/contrib/shutdown-prom` | Prometheus metrics |
| `shutdown-health` | `github.com/ubgo/shutdown/contrib/shutdown-health` | Auto-flip `ubgo/health` readiness on PreShutdown |
| `shutdown-nethttp` | `github.com/ubgo/shutdown/contrib/shutdown-nethttp` | `http.Server.Shutdown` registered handler |
| `shutdown-gin` / `-chi` / `-echo` / `-fiber` | `…/contrib/shutdown-<framework>` | Framework-server shutdown helpers |
| `shutdown-fx` | `github.com/ubgo/shutdown/contrib/shutdown-fx` | uber-go/fx lifecycle bridge |

(Adapters land in subsequent releases; v0.1.0 ships the core only.)

## Comparison

| Feature | uber-fx | oklog/run | tokio-graceful-shutdown | terminus | **`ubgo/shutdown`** |
|---------|:-------:|:---------:|:------------------------:|:--------:|:--------------------:|
| Phase-based ordering | ❌ | ❌ | ❌ | ❌ | **✅** |
| Parallel within phase | ❌ | partial | ✅ | ❌ | **✅** |
| Force-exit on second signal | ❌ | ❌ | ✅ | ❌ | **✅** |
| Watchdog hard-exit | ❌ | ❌ | ✅ | ❌ | **✅** |
| Observer pattern | ❌ | ❌ | ❌ | ❌ | **✅** |
| Native readiness drain | ❌ | ❌ | ❌ | ✅ | ✅ (contrib) |
| Actor (run+interrupt) pairs | ❌ | ✅ | partial | ❌ | **✅** |
| Reload signal hook | ❌ | ❌ | ❌ | ❌ | **✅** |
| Zero-dep core | ❌ | ✅ | ❌ | ❌ | **✅** |

## Compatibility

Requires Go 1.24 or later.

## License

Apache License 2.0. See [`LICENSE`](./LICENSE) and [`NOTICE`](./NOTICE).
