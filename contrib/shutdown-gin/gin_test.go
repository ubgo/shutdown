package shutdowngin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ubgo/shutdown"
)

func TestRegister_GracefulShutdown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	srv := httptest.NewUnstartedServer(r)
	srv.Start()
	t.Cleanup(srv.Close)

	mgr := shutdown.New(shutdown.WithLogger(shutdown.NoopLogger()), shutdown.WithBudget(2*time.Second))
	if err := Register(mgr, srv.Config); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestRegister_AllOptions(t *testing.T) {
	mgr := shutdown.New(shutdown.WithLogger(shutdown.NoopLogger()))
	srv := &http.Server{}
	if err := Register(mgr, srv,
		WithName("gin-api"),
		WithPhase(shutdown.PhaseStopAccepting),
		WithTimeout(time.Second),
	); err != nil {
		t.Fatalf("Register: %v", err)
	}
}
