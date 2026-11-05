package server_handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/evolution-foundation/evolution-go/pkg/metrics"
)

func newRouter(deps Dependencies) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewServerHandler(deps)
	r := gin.New()
	r.GET("/server/ok", h.ServerOk)
	r.GET("/health", h.Health)
	r.GET("/metrics", h.Metrics)
	return r
}

func get(r *gin.Engine, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func TestServerOkOnlyReportsTheProcessIsUp(t *testing.T) {
	// Mesmo com um banco fora do ar, /server/ok continua 200: é só liveness.
	r := newRouter(Dependencies{Checks: []Check{{Name: "database", Ping: func(context.Context) error { return errors.New("down") }}}})

	w := get(r, "/server/ok")
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"status":"ok"}` {
		t.Fatalf("status %d, corpo %s", w.Code, w.Body)
	}
}

func decodeHealth(t *testing.T, w *httptest.ResponseRecorder) HealthResponse {
	t.Helper()
	var out HealthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("resposta não é JSON: %v\n%s", err, w.Body)
	}
	return out
}

func TestHealthIsOkWhenEveryDatabaseResponds(t *testing.T) {
	r := newRouter(Dependencies{
		Checks: []Check{
			{Name: "database", Ping: func(context.Context) error { return nil }},
			{Name: "authDatabase", Ping: func(context.Context) error { return nil }},
		},
		Clients: func() ClientCounts { return ClientCounts{Total: 3, Connected: 2, LoggedIn: 2} },
	})

	w := get(r, "/health")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d, esperado 200: %s", w.Code, w.Body)
	}
	got := decodeHealth(t, w)
	if got.Status != "ok" || got.Checks["database"].Status != "up" || got.Checks["authDatabase"].Status != "up" {
		t.Fatalf("resposta inesperada: %+v", got)
	}
	if got.Clients != (HealthClients{Total: 3, Connected: 2, LoggedIn: 2}) {
		t.Fatalf("clientes = %+v", got.Clients)
	}
}

func TestHealthIs503AndHidesTheErrorWhenADatabaseFails(t *testing.T) {
	r := newRouter(Dependencies{
		Checks: []Check{
			{Name: "database", Ping: func(context.Context) error { return nil }},
			{Name: "authDatabase", Ping: func(context.Context) error {
				return errors.New("dial tcp 10.0.0.5:5432: connect: refused (user=evogo password=hunter2)")
			}},
		},
	})

	w := get(r, "/health")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d, esperado 503: %s", w.Code, w.Body)
	}
	got := decodeHealth(t, w)
	if got.Status != "unavailable" || got.Checks["authDatabase"].Status != "down" || got.Checks["database"].Status != "up" {
		t.Fatalf("resposta inesperada: %+v", got)
	}
	// O endpoint é público: nada do erro do driver pode sair na resposta.
	for _, leak := range []string{"hunter2", "10.0.0.5", "dial tcp"} {
		if strings.Contains(w.Body.String(), leak) {
			t.Errorf("a resposta vazou %q: %s", leak, w.Body)
		}
	}
}

func TestHealthGivesUpOnADatabaseThatNeverAnswers(t *testing.T) {
	r := newRouter(Dependencies{
		CheckTimeout: 50 * time.Millisecond,
		Checks: []Check{{Name: "database", Ping: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		}}},
	})

	started := time.Now()
	w := get(r, "/health")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d, esperado 503", w.Code)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("o health check ficou preso %s num banco que não responde", elapsed)
	}
}

func TestHealthWithoutDependenciesDoesNotPanic(t *testing.T) {
	w := get(newRouter(Dependencies{}), "/health")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d, esperado 200", w.Code)
	}
}

func TestMetricsExposeClientsPoolsAndProcessCounters(t *testing.T) {
	metrics.WebhookGivenUp.Inc()

	r := newRouter(Dependencies{
		Clients: func() ClientCounts { return ClientCounts{Total: 4, Connected: 3, LoggedIn: 2} },
		Pools: []Pool{
			{Name: "auth", Stats: func() sql.DBStats {
				return sql.DBStats{MaxOpenConnections: 25, OpenConnections: 7, InUse: 2, Idle: 5, WaitCount: 9, WaitDuration: 1500 * time.Millisecond}
			}},
			{Name: "users", Stats: func() sql.DBStats { return sql.DBStats{OpenConnections: 1, Idle: 1} }},
			{Name: "sem-estatistica"},
		},
	})

	w := get(r, "/metrics")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q", ct)
	}

	body := w.Body.String()
	for _, want := range []string{
		"evogo_clients 4\n",
		"evogo_clients_connected 3\n",
		"evogo_clients_logged_in 2\n",
		`evogo_db_open_connections{pool="auth"} 7`,
		`evogo_db_open_connections{pool="users"} 1`,
		`evogo_db_in_use_connections{pool="auth"} 2`,
		`evogo_db_wait_count_total{pool="auth"} 9`,
		`evogo_db_wait_seconds_total{pool="auth"} 1.5`,
		"# TYPE evogo_instance_reconnects_total counter",
		"# TYPE evogo_webhook_given_up_total counter",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("faltou %q em /metrics:\n%s", want, body)
		}
	}
	if strings.Contains(body, "sem-estatistica") {
		t.Errorf("pool sem estatística não deveria aparecer:\n%s", body)
	}
}
