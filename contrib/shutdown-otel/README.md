# shutdown-otel

> **Role: Observer.** Subscribes to `shutdown.Manager` events and emits
> [OpenTelemetry](https://opentelemetry.io/) spans for the shutdown
> sequence — one root, one per phase, one per handler.

## Install

```sh
go get github.com/ubgo/shutdown
go get github.com/ubgo/shutdown/contrib/shutdown-otel
```

## Use

```go
import (
    "github.com/ubgo/shutdown"
    shutdownotel "github.com/ubgo/shutdown/contrib/shutdown-otel"
    "go.opentelemetry.io/otel"
)

mgr := shutdown.New()

tracer := otel.Tracer("shutdown")
mgr.Subscribe(shutdownotel.Observer(tracer))
```

## Span hierarchy

```
shutdown                                (root)
└─ shutdown.phase.PreShutdown
└─ shutdown.phase.StopAccepting
   └─ shutdown.handler.http.Server
└─ shutdown.phase.CloseClients
   └─ shutdown.handler.db
   └─ shutdown.handler.redis
└─ shutdown.phase.FlushLogs
   └─ shutdown.handler.otel-flush
```

Each handler span carries:

- `name` (string)
- `phase` (string)
- `duration_ms` (int64)
- on error: `RecordError(err)` event + `SetStatus(codes.Error, ...)`

## License

Apache-2.0 — see [`LICENSE`](../../LICENSE).
