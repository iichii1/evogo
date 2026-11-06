package whatsmeow_service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/socket"
	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/evolution-foundation/evolution-go/pkg/runtimestate"
)

// dialAnswering abre o socket de verdade contra um servidor local que responde com esse status, para ter o erro
// exato que a whatsmeow devolve (ErrWithStatusCode não pode ser montado fora do pacote dela).
func dialAnswering(t *testing.T, status int) error {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) }))
	defer server.Close()
	fs := socket.NewFrameSocket(waLog.Noop, server.Client())
	fs.URL = "ws" + strings.TrimPrefix(server.URL, "http")
	err := fs.Connect(context.Background())
	if err == nil {
		t.Fatal("o servidor não deveria deixar abrir o socket")
	}
	return err
}

func TestOnlyPassingConnectErrorsAreRetried(t *testing.T) {
	refused := &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
	proxyDenied := &net.OpError{Op: "socks connect", Net: "tcp", Err: errors.New("username/password authentication failed")}
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"sem erro", nil, false},
		{"conexão recusada", fmt.Errorf("%w: %w", socket.ErrDialFailed, refused), true},
		{"servidor sem resposta", dialAnswering(t, http.StatusServiceUnavailable), true},
		{"gateway ruim", dialAnswering(t, http.StatusBadGateway), true},
		{"cliente desatualizado (405)", dialAnswering(t, http.StatusMethodNotAllowed), false},
		{"proibido (403)", dialAnswering(t, http.StatusForbidden), false},
		{"não autorizado (401)", dialAnswering(t, http.StatusUnauthorized), false},
		{"fim de arquivo", io.EOF, true},
		{"fim de arquivo dentro da negociação", fmt.Errorf("noise handshake failed: %w", io.ErrUnexpectedEOF), true},
		{"negociação sem resposta", fmt.Errorf("noise handshake failed: %w", whatsmeow.ErrHandshakeTimeout), true},
		{"socket fechado", socket.ErrSocketClosed, true},
		{"proxy recusou a senha", proxyDenied, false},
		{"outro erro", errors.New("device store deleted"), false},
	}
	for _, c := range cases {
		if got := isTransientConnectError(c.err); got != c.want {
			t.Errorf("%s: esperado %v, veio %v (%v)", c.name, c.want, got, c.err)
		}
	}
}

func TestTheRetryDelayDoublesUpToTheCap(t *testing.T) {
	want := []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 40 * time.Second, 80 * time.Second, 2 * time.Minute, 2 * time.Minute, 2 * time.Minute}
	for i, expected := range want {
		if got := connectRetryDelay(i + 1); got != expected {
			t.Errorf("tentativa extra %d: esperado %s, veio %s", i+1, expected, got)
		}
	}
	if len(want) != connectRetryLimit {
		t.Errorf("a tabela de esperas tem %d linhas e o limite é %d", len(want), connectRetryLimit)
	}
	if got := connectRetryDelay(0); got != 0 {
		t.Errorf("antes da primeira tentativa extra não há espera, veio %s", got)
	}
}

// runRetry roda connectWithRetry com um connect que devolve os erros da lista, em ordem, e depois nil.
func runRetry(errs []error, stopAfterWaits int) (result error, connects int, waits []time.Duration) {
	connect := func() error {
		connects++
		if connects <= len(errs) {
			return errs[connects-1]
		}
		return nil
	}
	wait := func(d time.Duration) bool {
		waits = append(waits, d)
		return stopAfterWaits < 0 || len(waits) <= stopAfterWaits
	}
	result = connectWithRetry(connect, wait, func(int, time.Duration, error) {})
	return
}

func TestConnectingTriesAgainAfterAPassingFailure(t *testing.T) {
	down := fmt.Errorf("%w: %w", socket.ErrDialFailed, &net.OpError{Op: "dial", Err: syscall.ENETUNREACH})

	if err, connects, waits := runRetry(nil, -1); err != nil || connects != 1 || len(waits) != 0 {
		t.Errorf("conectou de primeira: esperado 1 conexão sem espera, veio %v, %d, %v", err, connects, waits)
	}

	err, connects, waits := runRetry([]error{down, down}, -1)
	if err != nil || connects != 3 || len(waits) != 2 || waits[0] != 5*time.Second || waits[1] != 10*time.Second {
		t.Errorf("duas falhas e depois conecta: veio %v, %d conexões, esperas %v", err, connects, waits)
	}

	always := make([]error, 100)
	for i := range always {
		always[i] = down
	}
	err, connects, waits = runRetry(always, -1)
	if !errors.Is(err, socket.ErrDialFailed) || connects != 1+connectRetryLimit || len(waits) != connectRetryLimit {
		t.Errorf("sempre falha: esperado parar após %d conexões, veio %v, %d, %d esperas", 1+connectRetryLimit, err, connects, len(waits))
	}
}

func TestConnectingDoesNotRetryWhatCannotBeFixedByWaiting(t *testing.T) {
	outdated := dialAnswering(t, http.StatusMethodNotAllowed)
	err, connects, waits := runRetry([]error{outdated, outdated}, -1)
	if err == nil || connects != 1 || len(waits) != 0 {
		t.Errorf("405 não deveria ser repetido: veio %v, %d conexões, esperas %v", err, connects, waits)
	}
}

func TestStoppingTheSessionEndsTheRetries(t *testing.T) {
	down := fmt.Errorf("%w: %w", socket.ErrDialFailed, &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED})
	always := []error{down, down, down, down}
	err, connects, waits := runRetry(always, 1)
	if err == nil || connects != 2 || len(waits) != 2 {
		t.Errorf("parada na segunda espera: esperado parar com o erro após 2 conexões, veio %v, %d, %v", err, connects, waits)
	}
}

func TestAnInstanceInTheRetryQueueIsNotStartedAgain(t *testing.T) {
	var none whatsmeowService
	if none.connectingWithRetry("a") {
		t.Error("sem o mapa, nada está tentando")
	}
	w := whatsmeowService{connectRetrying: runtimestate.New[string, bool]()}
	w.connectRetrying.Set("a", true)
	if !w.connectingWithRetry("a") || w.connectingWithRetry("b") {
		t.Error("só a instância que está na fila deve constar")
	}
	w.connectRetrying.Delete("a")
	if w.connectingWithRetry("a") {
		t.Error("saiu da fila, não deve constar")
	}
}
