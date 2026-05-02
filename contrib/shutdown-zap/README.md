# shutdown-zap

> **Role: Observer.** Subscribes to `shutdown.Manager` events and emits
> structured log lines via [`go.uber.org/zap`](https://github.com/uber-go/zap).

The core's internal Logger uses `log/slog` by default; this adapter is for
operators who prefer zap's ecosystem (zap fields, levels, encoders) and
want a side channel of detailed event logs that flow through their existing
zap pipeline.

## Install

```sh
go get github.com/ubgo/shutdown
go get github.com/ubgo/shutdown/contrib/shutdown-zap
```

## Use

```go
import (
    "github.com/ubgo/shutdown"
    shutdownzap "github.com/ubgo/shutdown/contrib/shutdown-zap"
    "go.uber.org/zap"
)

mgr := shutdown.New()

logger, _ := zap.NewProduction()
mgr.Subscribe(shutdownzap.Observer(logger))
```

## What you get

| Event | Log line | Level |
|-------|----------|-------|
| Signal arrived | `shutdown: signal received` | INFO |
| Phase started | `shutdown: phase start` | INFO |
| Phase ended | `shutdown: phase end` | INFO |
| Handler started | `shutdown: handler start` | INFO |
| Handler succeeded | `shutdown: handler end` | INFO |
| Handler failed | `shutdown: handler failed` | ERROR |
| Shutdown done (clean) | `shutdown: completed` | INFO |
| Shutdown done (errors) | `shutdown: completed with errors` | ERROR |

Every line includes structured fields for `phase`, `name`, `duration`, `error`.

## License

Apache-2.0 — see [`LICENSE`](../../LICENSE).
