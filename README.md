# shutdown

Phased, parallel-within-phase, observable graceful shutdown manager for long-running Go services. Designed for Kubernetes, systemd, and any orchestrator that delivers SIGTERM with a grace period.

Zero third-party dependencies in the core. Eight adapter modules — five HTTP frameworks (`nethttp`, `gin`, `chi`, `echo`, `fiber`) and three observers (`zap`, `otel`, `prom`) — ship under `contrib/`. Each contrib is its own Go module so importing one doesn't pull in the others.

## Install

```sh
go get github.com/ubgo/shutdown
```

## Quick start — production-ready in 30 lines

```go
package main

import (
    "context"
    "log"
    "net/http"
    "time"

    "github.com/ubgo/shutdown"
    shutdownnethttp "github.com/ubgo/shutdown/contrib/shutdown-nethttp"
)

func main() {
    mgr := shutdown.New(shutdown.WithBudget(30 * time.Second))

    // 1. HTTP server stops accepting first.
    srv := &http.Server{Addr: ":8080", Handler: yourMux()}
    _ = shutdownnethttp.Register(mgr, srv)

    // 2. DB / cache / queue clients close after traffic drains.
    _ = mgr.Register("db",    closeFn(db.Close),    shutdown.WithPhase(shutdown.PhaseCloseClients))
    _ = mgr.Register("redis", closeFn(rdb.Close),   shutdown.WithPhase(shutdown.PhaseCloseClients))
    _ = mgr.Register("nats",  closeFn(nc.Drain),    shutdown.WithPhase(shutdown.PhaseDrainTraffic))

    // 3. OpenTelemetry flushes last so the prior phases' spans actually leave.
    _ = mgr.Register("otel",  tp.Shutdown,          shutdown.WithPhase(shutdown.PhaseFlushLogs))

    go func() { _ = srv.ListenAndServe() }()

    if err := mgr.Listen(context.Background()); err != nil {
        log.Fatal(err)
    }
}

// closeFn adapts a `func() error` Close method into the manager's
// `func(ctx) error` HandlerFunc shape.
func closeFn(fn func() error) shutdown.HandlerFunc {
    return func(_ context.Context) error { return fn() }
}
```

`Listen` blocks until SIGTERM/SIGINT, then runs every registered handler in phase order. Handlers in the same phase run in parallel. The whole thing is bounded by `WithBudget`; a watchdog hard-exits if budget plus grace expires.

## Strategy: which phase does each thing go in?

The seven predefined phases match the typical Kubernetes preStop drain pattern. Lower phases run first. Within a phase, handlers run in parallel — order them across phases, not within.

| Phase | Value | What goes here | Common mistakes |
|-------|-------|----------------|-----------------|
| `PhasePreShutdown` | -100 | Flip readiness to Down so the load balancer stops sending. | Don't close anything yet; the LB still has a few seconds of in-flight traffic. |
| `PhaseStopAccepting` | 0 | Close HTTP / gRPC listeners. | Don't drain in-flight here — that's the next phase. `srv.Shutdown` already does both, but contribs put it here so dependencies live to serve those drains. |
| `PhaseDrainTraffic` | 100 | Wait for in-flight work to finish. NATS `Drain()`. Worker pools that own queue items. | Closing the DB here will fail in-flight requests. |
| `PhaseFlushQueues` | 200 | Flush async producers (Kafka, log shippers, batchers). | Don't close the underlying client until next phase. |
| `PhaseCloseClients` | 300 | Close DB / cache / messaging client connections. | Putting OTEL flush here loses spans for prior phases. |
| `PhaseFlushLogs` | 400 | Flush logs and traces last so prior errors actually leave the process. | Closing the logger before this phase silences the rest of the shutdown. |
| `PhasePostShutdown` | 500 | Final cleanup, exit-code reporting. | (Most apps don't need this.) |

Phases are plain `int` — pass any value to `WithPhase` if you need finer-grained ordering between the predefined ones.

## Recipes

### HTTP server with database and Redis

```go
mgr := shutdown.New(shutdown.WithBudget(30 * time.Second))

srv := &http.Server{Addr: ":8080", Handler: r}
_ = shutdownnethttp.Register(mgr, srv)

_ = mgr.Register("db",    closeFn(db.Close),  shutdown.WithPhase(shutdown.PhaseCloseClients))
_ = mgr.Register("redis", closeFn(rdb.Close), shutdown.WithPhase(shutdown.PhaseCloseClients))

go func() { _ = srv.ListenAndServe() }()
_ = mgr.Listen(ctx)
```

`db` and `redis` close in parallel (same phase), but only after `srv.Shutdown` has fully returned in the previous phase.

### Background worker (actor pattern)

When the run loop and the cancel mechanism are distinct goroutines:

```go
stop := make(chan struct{})

handle, _ := mgr.RegisterActor("worker", func(_ error) {
    close(stop) // tell the run loop to exit
}, shutdown.WithActorPhase(shutdown.PhaseDrainTraffic))

go func() {
    err := worker.Run(stop) // blocks until stop is closed
    handle.Done(err)         // tells the manager the actor finished
}()
```

`mgr.Listen` will fire the interrupt, then wait up to `WithActorTimeout` for `handle.Done`.

### Programmatic shutdown — no OS signals

Tests, panic-recovery middleware, custom `/admin/shutdown` endpoints, and health-failure paths can drive shutdown directly:

```go
mux.HandleFunc("/admin/shutdown", func(w http.ResponseWriter, r *http.Request) {
    if !authorized(r) { w.WriteHeader(http.StatusForbidden); return }
    go func() {
        _ = mgr.Shutdown(context.Background()) // same execution path as a signal
    }()
    w.WriteHeader(http.StatusAccepted)
})
```

### Reload signal (SIGHUP / SIGUSR1) — Gunicorn-style

```go
mgr.OnSignal(syscall.SIGHUP, func(ctx context.Context, _ os.Signal) {
    if err := config.Reload(); err != nil {
        log.Println("reload failed:", err)
    }
})
mgr.OnSignal(syscall.SIGUSR1, func(ctx context.Context, _ os.Signal) {
    rotateLogFile()
})
```

The hook fires; shutdown is NOT triggered. The hooked signal is automatically added to the listened set — no need to also pass it to `WithSignals`.

### Stack three observers at once

The observer pattern is composable: subscribe as many as you like.

```go
mgr.Subscribe(shutdownzap.Observer(zapLogger))
mgr.Subscribe(shutdownotel.Observer(tracer))
mgr.Subscribe(shutdownprom.Observer(promMetrics))
```

You get structured logs, distributed traces, and metrics from a single shutdown sequence — without any of the contribs knowing about each other.

### Custom observer for ad-hoc telemetry

Don't want a whole contrib for a one-off webhook? Subscribe inline:

```go
mgr.Subscribe(shutdown.Observer{
    OnComplete: func(total time.Duration, err error) {
        status := "success"
        if err != nil { status = "fail" }
        _ = postWebhook(map[string]any{
            "event":    "shutdown_complete",
            "duration": total.String(),
            "status":   status,
        })
    },
})
```

## Error handling

Every handler can return an error. By default the manager runs every phase to completion and returns `errors.Join(...)` of the lot:

```go
err := mgr.Shutdown(ctx)
if err != nil {
    var pe *shutdown.PanicError
    if errors.As(err, &pe) {
        log.Printf("handler %q panicked: %v", pe.Name, pe.Value)
    }
    log.Printf("shutdown errors: %v", err)
}
```

A panicking handler is converted to a `*PanicError` (wrapped in the aggregate) rather than crashing the shutdown goroutine.

Switch to fail-fast with `WithErrorPolicy(StopOnError)` if you'd rather skip subsequent phases on the first failure.

### Exit codes

By default `Listen` returns the error to your caller and you control `os.Exit`. To make the manager exit for you:

```go
mgr := shutdown.New(
    shutdown.WithExitOnComplete(0, 1), // success, failure
)
_ = mgr.Listen(ctx) // never returns to caller; os.Exit(0|1) at end
```

A second SIGTERM during shutdown calls `os.Exit(130)` immediately — the operator's escape hatch when something hangs. Tune via `WithForceOnSecondSignal(true, 130)` (or disable with `false, 0`).

## Watchdog

`WithBudget(d)` is the wall-clock budget across all phases. After budget plus a 1-second grace period (`WithWatchdogGrace`), the watchdog calls `os.Exit(failureCode)` even if handlers are still mid-execution. Stuck handler names are logged so you have something to grep for.

```go
mgr := shutdown.New(
    shutdown.WithBudget(25*time.Second),    // soft limit
    shutdown.WithWatchdogGrace(2*time.Second), // after which: os.Exit
    shutdown.WithExitOnComplete(0, 1),
)
```

This is the answer to "k8s SIGKILLs us at `terminationGracePeriodSeconds`": set the budget a few seconds shorter and exit clean before the kernel is involved.

### Tuning the timeout cascade

When you wrap a library that has its own internal deadline (e.g. `gocron.WithStopTimeout`, `redis.Options.PoolTimeout`, gRPC client `WithTimeout`), three layers compete:

```
[underlying lib's own deadline]   <   per-handler shutdown.WithTimeout   <   manager.WithBudget
```

Each layer must outlive the one below it. Reverse the order and you get bugs:

| Mistake | What happens |
|---------|--------------|
| `WithBudget` < library's deadline | Watchdog hard-exits before the library finishes its drain. In-flight work killed; orchestrator sees a non-zero exit. |
| `WithTimeout` < library's deadline | Manager cancels the per-handler context mid-call. The library's `Shutdown` returns `ctx.Err`; whatever it was draining keeps running in a goroutine no one is waiting on. |
| Both equal | Race condition — sometimes works, sometimes the watchdog wins. |

For typical service shapes:

| Service | Library deadline | `WithTimeout` | `WithBudget` |
|---------|------------------|---------------|--------------|
| HTTP API + DB pool | n/a | 10–15s per handler | 30s |
| HTTP API + slow downstream calls | call-level timeout (5–30s) | call timeout + 1s | 60s |
| gocron with hour-long jobs | `WithStopTimeout(24h)` | `24h 30m` | `25h` |
| Batch job that must not be interrupted | n/a | per-handler enough for cleanup | `WithBudget(0)` (disable; rely on orchestrator) |

The 24-hour gocron case is shown end-to-end in [`shutdown-examples/11-cron-gocron`](https://github.com/ubgo/shutdown-examples/tree/main/11-cron-gocron).

## Adapters

Adapter modules ship as separate Go modules under `contrib/`. Import only the ones you use; each pulls only its own dependencies.

| Adapter | Module path | Role |
|---------|-------------|------|
| [`shutdown-nethttp`](contrib/shutdown-nethttp) | `github.com/ubgo/shutdown/contrib/shutdown-nethttp` | `*http.Server.Shutdown` registered as a phase handler |
| [`shutdown-gin`](contrib/shutdown-gin) | `github.com/ubgo/shutdown/contrib/shutdown-gin` | Same, for the `*http.Server` wrapping a Gin engine |
| [`shutdown-chi`](contrib/shutdown-chi) | `github.com/ubgo/shutdown/contrib/shutdown-chi` | Same, for the `*http.Server` wrapping a Chi router |
| [`shutdown-echo`](contrib/shutdown-echo) | `github.com/ubgo/shutdown/contrib/shutdown-echo` | `*echo.Echo.Shutdown` (Echo owns its server) |
| [`shutdown-fiber`](contrib/shutdown-fiber) | `github.com/ubgo/shutdown/contrib/shutdown-fiber` | `*fiber.App.ShutdownWithContext` |
| [`shutdown-zap`](contrib/shutdown-zap) | `github.com/ubgo/shutdown/contrib/shutdown-zap` | Observer that emits structured logs via `go.uber.org/zap` |
| [`shutdown-otel`](contrib/shutdown-otel) | `github.com/ubgo/shutdown/contrib/shutdown-otel` | Observer that emits OpenTelemetry spans (root + phase + handler) |
| [`shutdown-prom`](contrib/shutdown-prom) | `github.com/ubgo/shutdown/contrib/shutdown-prom` | Observer that exports Prometheus metrics |

Runnable end-to-end demos for each pattern live in [`ubgo/shutdown-examples`](https://github.com/ubgo/shutdown-examples).

## Comparison

| Feature | uber-fx | oklog/run | tokio-graceful-shutdown | terminus | **`ubgo/shutdown`** |
|---------|:-------:|:---------:|:------------------------:|:--------:|:--------------------:|
| Phase-based ordering | ❌ | ❌ | ❌ | ❌ | **✅** |
| Parallel within phase | ❌ | partial | ✅ | ❌ | **✅** |
| Force-exit on second signal | ❌ | ❌ | ✅ | ❌ | **✅** |
| Watchdog hard-exit | ❌ | ❌ | ✅ | ❌ | **✅** |
| Observer pattern | ❌ | ❌ | ❌ | ❌ | **✅** |
| Actor (run+interrupt) pairs | ❌ | ✅ | partial | ❌ | **✅** |
| Reload signal hook | ❌ | ❌ | ❌ | ❌ | **✅** |
| Panic in handler → aggregate err | ❌ | ❌ | ❌ | ❌ | **✅** |
| Zero-dep core | ❌ | ✅ | ❌ | ❌ | **✅** |

## Compatibility

Requires Go 1.24 or later.

## License

Apache License 2.0. See [`LICENSE`](./LICENSE) and [`NOTICE`](./NOTICE).
