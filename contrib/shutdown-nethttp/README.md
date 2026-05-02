# shutdown-nethttp

> **Role: HTTP server adapter.** Registers a `*http.Server.Shutdown(ctx)`
> as a phase handler on a [`shutdown.Manager`](https://github.com/ubgo/shutdown).
> Default phase: `PhaseStopAccepting` — listeners stop before dependencies close.

## Install

```sh
go get github.com/ubgo/shutdown
go get github.com/ubgo/shutdown/contrib/shutdown-nethttp
```

## Quick start

```go
package main

import (
    "context"
    "net/http"

    "github.com/ubgo/shutdown"
    shutdownnethttp "github.com/ubgo/shutdown/contrib/shutdown-nethttp"
)

func main() {
    mgr := shutdown.New()

    mux := http.NewServeMux()
    srv := &http.Server{Addr: ":8080", Handler: mux}

    if err := shutdownnethttp.Register(mgr, srv); err != nil {
        panic(err)
    }

    go func() { _ = srv.ListenAndServe() }()

    _ = mgr.Listen(context.Background())
}
```

## Options

| Option | Default | Purpose |
|--------|---------|---------|
| `WithName(s)` | `"http.Server"` | Override handler name in the manager |
| `WithPhase(p)` | `PhaseStopAccepting` | Place the shutdown in a different phase |
| `WithTimeout(d)` | `10s` | Cap time spent inside `srv.Shutdown` |

## Behaviour

- `srv.Shutdown` is called with the per-handler context. Cancellation propagates.
- `http.ErrServerClosed` is treated as success.
- Other errors propagate to the manager's aggregated error.

## License

Apache-2.0 — see [`LICENSE`](../../LICENSE) at the repository root.
