# AGENTS.md — codebase map for AI agents

Read this first. Orientation map for `ubgo/shutdown` so a fresh agent knows what every part does and where to change things, without reading every file.

## What this repo is

`ubgo/shutdown` is a **phased, parallel-within-phase, observable graceful-shutdown manager** for Go services. You register named handlers (and "actors" — long-running goroutines) into ordered phases; on a signal (or a programmatic trigger) the `Manager` runs each phase in order, handlers within a phase in parallel, with per-handler timeouts, an overall budget, a watchdog hard-exit, force-exit on a second signal, error aggregation, and observer hooks for telemetry. The core has **zero third-party dependencies**; framework/telemetry integrations live in `contrib/`. See `README.md` for the pitch, `doc.go` for the package overview.

## Modules

| Path | Module | Role | Deps |
|---|---|---|---|
| `.` | `github.com/ubgo/shutdown` | Core manager. | stdlib only (a `zero-dep-check` CI gate enforces this) |
| `contrib/shutdown-{nethttp,gin,chi,echo,fiber}` | each own module | HTTP-server shutdown adapters. | the target framework |
| `contrib/shutdown-{otel,prom,zap}` | each own module | Observer/Logger adapters for OpenTelemetry, Prometheus, zap. | the target lib |

Go 1.24. Each contrib is a separate module so the core stays dependency-free.

## Core files — what each owns

| File | Responsibility |
|---|---|
| `doc.go` | Package overview godoc. |
| `types.go` | `HandlerFunc`, `Phase` (+ the predefined phase constants), `ErrorPolicy`, `Logger` interface, `Observer`, `RunFunc`/`InterruptFunc`. |
| `manager.go` | `Manager` — the central coordinator: `New`, `Register`, `Subscribe`, `Listen` (signal-driven), `Shutdown` (programmatic), `OnSignal`. |
| `register.go` | `Register`/`RegisterOption` (`WithPhase`, `WithTimeout`, …) and the internal `registration`. |
| `runner.go` | Phase execution: parallel-within-phase by default, serial when opted in; per-handler ctx = min(handler timeout, remaining budget); error aggregation. |
| `actor.go` | "Actors" — register a long-running `RunFunc` + `InterruptFunc` pair that the manager interrupts and waits for, unified into the same phase machinery. |
| `watchdog.go` | The hard-exit safety net: after budget + grace, `os.Exit` with the stuck-handler names logged. |
| `observer.go` | Observer fan-out (`OnSignal`/`OnPhaseStart`/`OnHandlerEnd`/`OnComplete`, …) consumed by `contrib/shutdown-otel`/`-prom`. |
| `options.go` | `Option` / `config` — `WithBudget`, `WithLogger`, `WithSignals`, `WithForceOnSecondSignal`, `WithExitOnComplete`, `WithErrorPolicy`, `WithSerial`. |
| `logger.go` | The minimal `Logger` interface's noop default (real loggers plug in via `WithLogger` / `contrib/shutdown-zap`). |

## The flow to understand

`manager.go:Listen` (or `Shutdown`) → sort registrations into phase buckets (`bucketsByPhase`) → for each phase ascending: fire `OnPhaseStart`, run handlers (`runner.go:runPhase`, parallel unless `WithSerial`), aggregate errors per `ErrorPolicy`, fire `OnPhaseEnd` → `OnComplete` with `errors.Join`. A second signal force-exits; the `watchdog` hard-exits if the budget is blown. Phases run logs/flush **last** by phase number so earlier errors are recorded.

## Conventions

- **Zero third-party deps in core** — the `zero-dep-check` task/CI gate fails if `go.mod` gains a non-stdlib require. Framework/telemetry code goes in `contrib/`.
- **Race detector mandatory**; high coverage. `task ci` runs everything.
- **No panics in libraries** — handlers' panics are recovered into a `PanicError` and surfaced in the aggregate.
- Comments explain *why*, not *what*.

## Running

```sh
task ci            # fmt + vet + race tests + coverage + zero-dep-check
task test:race
task zero-dep-check
```

## Where to look for X

- "Add a phase / change ordering" → `types.go` (phase constants) + `WithPhase` in `register.go`.
- "Wire it to my HTTP framework" → `contrib/shutdown-<framework>`.
- "Emit shutdown telemetry" → `observer.go` + `contrib/shutdown-otel` / `-prom`.
- "Force-exit / watchdog behavior" → `manager.go` (`runShutdownWithForceWatch`) + `watchdog.go`.
