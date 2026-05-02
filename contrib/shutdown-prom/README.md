# shutdown-prom

> **Role: Observer.** Subscribes to `shutdown.Manager` events and exports
> [Prometheus](https://prometheus.io/) metrics for shutdown phases + handlers.

## Install

```sh
go get github.com/ubgo/shutdown
go get github.com/ubgo/shutdown/contrib/shutdown-prom
```

## Use

```go
import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/ubgo/shutdown"
    shutdownprom "github.com/ubgo/shutdown/contrib/shutdown-prom"
)

mgr := shutdown.New()

m := shutdownprom.NewMetrics(prometheus.DefaultRegisterer)
mgr.Subscribe(shutdownprom.Observer(m))
```

Pass `nil` to either to default to `prometheus.DefaultRegisterer`.

## Metrics exported

| Metric | Type | Labels |
|--------|------|--------|
| `shutdown_phase_duration_seconds` | Histogram | `phase` |
| `shutdown_handler_duration_seconds` | Histogram | `phase`, `name`, `status` |
| `shutdown_handlers_total` | Counter | `phase`, `name`, `status` |

`status` is `ok` or `error`. Histograms use `prometheus.DefBuckets`.

## Sample queries

```promql
# Total wall-clock time spent shutting down, by phase
sum(rate(shutdown_phase_duration_seconds_sum[1h])) by (phase)

# Handlers that have failed at shutdown in the last hour
increase(shutdown_handlers_total{status="error"}[1h])
```

## License

Apache-2.0 — see [`LICENSE`](../../LICENSE).
