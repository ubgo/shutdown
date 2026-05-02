# shutdown-echo

> **Role: HTTP server adapter.** Registers `e.Shutdown(ctx)` directly on
> an `*echo.Echo`, on a [`shutdown.Manager`](https://github.com/ubgo/shutdown).

Unlike gin/chi, Echo owns its server, so pass the engine directly:

```go
import (
    "github.com/labstack/echo/v4"
    "github.com/ubgo/shutdown"
    shutdownecho "github.com/ubgo/shutdown/contrib/shutdown-echo"
)

mgr := shutdown.New()

e := echo.New()
_ = shutdownecho.Register(mgr, e)

go func() { _ = e.Start(":8080") }()
_ = mgr.Listen(nil)
```

## Options

| Option | Default | Purpose |
|--------|---------|---------|
| `WithName(s)` | `"echo.Server"` | Handler name override |
| `WithPhase(p)` | `PhaseStopAccepting` | Phase override |
| `WithTimeout(d)` | `10s` | Time cap |

## License

Apache-2.0 — see [`LICENSE`](../../LICENSE).
