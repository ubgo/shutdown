# shutdown-fiber

> **Role: HTTP server adapter.** Registers `app.ShutdownWithContext(ctx)`
> on a `*fiber.App`, on a [`shutdown.Manager`](https://github.com/ubgo/shutdown).

Fiber sits on fasthttp; its `ShutdownWithContext` is wired directly:

```go
import (
    "github.com/gofiber/fiber/v2"
    "github.com/ubgo/shutdown"
    shutdownfiber "github.com/ubgo/shutdown/contrib/shutdown-fiber"
)

mgr := shutdown.New()
app := fiber.New()

_ = shutdownfiber.Register(mgr, app)

go func() { _ = app.Listen(":8080") }()
_ = mgr.Listen(nil)
```

## Options

| Option | Default | Purpose |
|--------|---------|---------|
| `WithName(s)` | `"fiber.App"` | Handler name override |
| `WithPhase(p)` | `PhaseStopAccepting` | Phase override |
| `WithTimeout(d)` | `10s` | Time cap |

## License

Apache-2.0 — see [`LICENSE`](../../LICENSE).
