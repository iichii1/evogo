package webhook_producer

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	"github.com/evolution-foundation/evolution-go/pkg/metrics"
)

type counters struct{ delivered, attemptFailures, givenUp uint64 }

func snapshot() counters {
	return counters{
		delivered:       metrics.WebhookDelivered.Value(),
		attemptFailures: metrics.WebhookAttemptFailures.Value(),
		givenUp:         metrics.WebhookGivenUp.Value(),
	}
}

func newProducer(t *testing.T) (*webhookProducer, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{LogDirectory: dir}
	return &webhookProducer{loggerWrapper: logger_wrapper.NewLoggerManager(cfg)}, dir
}

func readLog(t *testing.T, dir, instance string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, instance, "instance.log"))
	if err != nil {
		t.Fatalf("sem log da instância: %v", err)
	}
	return string(raw)
}

func TestWebhookCountsDeliveryAndFailedAttempts(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= 2 {
			http.Error(w, "indisponível", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p, _ := newProducer(t)
	before := snapshot()
	p.sendWebhookWithRetry(srv.URL+"/hook", []byte(`{}`), 5, time.Millisecond, "i1")
	after := snapshot()

	if got := after.delivered - before.delivered; got != 1 {
		t.Errorf("entregas = %d, esperado 1", got)
	}
	if got := after.attemptFailures - before.attemptFailures; got != 2 {
		t.Errorf("tentativas que falharam = %d, esperado 2", got)
	}
	if got := after.givenUp - before.givenUp; got != 0 {
		t.Errorf("desistências = %d, esperado 0", got)
	}
}

func TestWebhookCountsGivingUpAfterTheLastAttempt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "fora do ar", http.StatusBadGateway)
	}))
	defer srv.Close()

	p, _ := newProducer(t)
	before := snapshot()
	p.sendWebhookWithRetry(srv.URL, []byte(`{}`), 3, time.Millisecond, "i1")
	after := snapshot()

	if got := after.attemptFailures - before.attemptFailures; got != 3 {
		t.Errorf("tentativas que falharam = %d, esperado 3", got)
	}
	if got := after.givenUp - before.givenUp; got != 1 {
		t.Errorf("desistências = %d, esperado 1", got)
	}
	if got := after.delivered - before.delivered; got != 0 {
		t.Errorf("entregas = %d, esperado 0", got)
	}
}

// A URL do webhook costuma levar um token (na query ou no caminho); ele não pode
// ir parar no log da instância, nem em caso de sucesso, nem de falha.
func TestWebhookLogsNeverContainTheURLSecrets(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			http.Error(w, "erro", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p, dir := newProducer(t)
	p.sendWebhookWithRetry(srv.URL+"/hooks/CAMINHO-SECRETO?token=QUERY-SECRETA", []byte(`{}`), 3, time.Millisecond, "i1")

	// Servidor que nem aceita a conexão: o erro do cliente HTTP repete a URL inteira.
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close()
	p.sendWebhookWithRetry(deadURL+"/hooks/CAMINHO-SECRETO?token=QUERY-SECRETA", []byte(`{}`), 2, time.Millisecond, "i1")

	log := readLog(t, dir, "i1")
	for _, secret := range []string{"CAMINHO-SECRETO", "QUERY-SECRETA"} {
		if strings.Contains(log, secret) {
			t.Errorf("o log da instância contém %q:\n%s", secret, log)
		}
	}
	if !strings.Contains(log, "webhook sent successfully") || !strings.Contains(log, "webhook failed after maximum retries") {
		t.Errorf("o log deveria registrar a entrega e a desistência:\n%s", log)
	}
}
