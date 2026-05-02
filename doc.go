// Package shutdown is a phased, parallel-within-phase, observable graceful
// shutdown manager for long-running Go services.
//
// The package has zero third-party dependencies. Logger, OTEL, Prometheus,
// health, and HTTP framework integrations live in adapter modules under
// contrib/.
//
// Typical use:
//
//	mgr := shutdown.New(
//	    shutdown.WithBudget(30 * time.Second),
//	)
//	mgr.Register("http", srv.Shutdown,    shutdown.WithPhase(shutdown.PhaseStopAccepting))
//	mgr.Register("nats", natsConn.Drain,  shutdown.WithPhase(shutdown.PhaseDrainTraffic))
//	mgr.Register("db",   db.Close,        shutdown.WithPhase(shutdown.PhaseCloseClients))
//	mgr.Register("otel", otelP.Shutdown,  shutdown.WithPhase(shutdown.PhaseFlushLogs))
//
//	if err := mgr.Listen(ctx); err != nil {
//	    log.Fatal(err)
//	}
//
// See the README and the companion examples repo at
// github.com/ubgo/shutdown-examples for k8s preStop, drain, OTEL tracing,
// and worker-actor patterns.
package shutdown
