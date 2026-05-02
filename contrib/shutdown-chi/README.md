# shutdown-chi

> **Role: HTTP server adapter.** Registers `srv.Shutdown(ctx)` for the
> `*http.Server` wrapping a Chi router, on a [`shutdown.Manager`](https://github.com/ubgo/shutdown).

```go
import (
    "net/http"

    "github.com/go-chi/chi/v5"
    "github.com/ubgo/shutdown"
    shutdownchi "github.com/ubgo/shutdown/contrib/shutdown-chi"
)

mgr := shutdown.New()

r := chi.NewRouter()
srv := &http.Server{Addr: ":8080", Handler: r}

_ = shutdownchi.Register(mgr, srv)
```

## Options

| Option | Default | Purpose |
|--------|---------|---------|
| `WithName(s)` | `"chi.Server"` | Handler name override |
| `WithPhase(p)` | `PhaseStopAccepting` | Phase override |
| `WithTimeout(d)` | `10s` | Time cap |

## License

Apache-2.0 — see [`LICENSE`](../../LICENSE).
