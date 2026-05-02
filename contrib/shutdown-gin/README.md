# shutdown-gin

> **Role: HTTP server adapter.** Registers `srv.Shutdown(ctx)` for the
> `*http.Server` wrapping a Gin engine, on a [`shutdown.Manager`](https://github.com/ubgo/shutdown).

Gin engines don't own a server — wrap your `gin.Engine` in a `*http.Server` you control:

```go
package main

import (
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/ubgo/shutdown"
    shutdowngin "github.com/ubgo/shutdown/contrib/shutdown-gin"
)

func main() {
    mgr := shutdown.New()

    r := gin.Default()
    srv := &http.Server{Addr: ":8080", Handler: r}

    _ = shutdowngin.Register(mgr, srv)
    go func() { _ = srv.ListenAndServe() }()
    _ = mgr.Listen(nil)
}
```

## Options

| Option | Default | Purpose |
|--------|---------|---------|
| `WithName(s)` | `"gin.Server"` | Override handler name |
| `WithPhase(p)` | `PhaseStopAccepting` | Place the shutdown in a specific phase |
| `WithTimeout(d)` | `10s` | Cap time spent inside `srv.Shutdown` |

## License

Apache-2.0 — see [`LICENSE`](../../LICENSE).
