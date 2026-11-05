package websocket_producer

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	"github.com/gorilla/websocket"
)

func newTestProducer(t *testing.T) *websocketProducer {
	t.Helper()
	return NewWebsocketProducer(logger_wrapper.NewLoggerManager(&config.Config{LogDirectory: t.TempDir()}))
}

// dial sobe um servidor de teste que registra a conexão no producer e devolve o
// lado cliente. instanceID vazio registra como broadcast.
func dial(t *testing.T, p *websocketProducer, instanceID string) *websocket.Conn {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWs(w, r, instanceID, p)
	}))
	t.Cleanup(srv.Close)

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condição não foi atingida a tempo")
}

// Regressão de #99: várias goroutines chamando Produce para a mesma conexão
// davam panic "concurrent write to websocket connection". Rode com -race.
func TestProduceConcurrentWritesDoNotPanic(t *testing.T) {
	p := newTestProducer(t)
	instanceClient := dial(t, p, "inst-1")
	broadcastClient := dial(t, p, "")
	waitFor(t, func() bool {
		p.clientsMux.RLock()
		defer p.clientsMux.RUnlock()
		return len(p.clients) == 1 && len(p.broadcast) == 1
	})

	const senders, perSender = 20, 25
	var wg sync.WaitGroup
	for i := 0; i < senders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perSender; j++ {
				if err := p.Produce("message", []byte(`{"k":"v"}`), "inst-1", ""); err != nil {
					t.Errorf("Produce: %v", err)
					return
				}
			}
		}()
	}

	read := func(c *websocket.Conn, name string) {
		for n := 0; n < senders*perSender; n++ {
			_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
			var m map[string]interface{}
			if err := c.ReadJSON(&m); err != nil {
				t.Errorf("%s: leitura %d: %v", name, n, err)
				return
			}
			if m["queue"] != "message" {
				t.Errorf("%s: queue = %v", name, m["queue"])
				return
			}
		}
	}
	var rg sync.WaitGroup
	rg.Add(2)
	go func() { defer rg.Done(); read(instanceClient, "instância") }()
	go func() { defer rg.Done(); read(broadcastClient, "broadcast") }()

	wg.Wait()
	rg.Wait()
}

// A conexão antiga que cai não pode remover a nova que a substituiu.
func TestOldConnectionDoesNotRemoveReplacement(t *testing.T) {
	p := newTestProducer(t)
	first := dial(t, p, "inst-1")
	waitFor(t, func() bool {
		p.clientsMux.RLock()
		defer p.clientsMux.RUnlock()
		return p.clients["inst-1"] != nil
	})
	oldClient := func() *wsClient {
		p.clientsMux.RLock()
		defer p.clientsMux.RUnlock()
		return p.clients["inst-1"]
	}()

	second := dial(t, p, "inst-1")
	waitFor(t, func() bool {
		p.clientsMux.RLock()
		defer p.clientsMux.RUnlock()
		return p.clients["inst-1"] != nil && p.clients["inst-1"] != oldClient
	})

	first.Close() // a conexão antiga cai
	time.Sleep(200 * time.Millisecond)

	p.clientsMux.RLock()
	remaining := p.clients["inst-1"]
	p.clientsMux.RUnlock()
	if remaining == nil {
		t.Fatal("a conexão nova foi removida quando a antiga caiu")
	}

	if err := p.Produce("message", []byte(`{}`), "inst-1", ""); err != nil {
		t.Fatal(err)
	}
	_ = second.SetReadDeadline(time.Now().Add(3 * time.Second))
	var m map[string]interface{}
	if err := second.ReadJSON(&m); err != nil {
		t.Fatalf("a conexão nova não recebeu o evento: %v", err)
	}
}

// Um cliente da instância com falha não pode impedir a entrega aos broadcast.
func TestProduceStillReachesBroadcastWhenInstanceClientFails(t *testing.T) {
	p := newTestProducer(t)
	inst := dial(t, p, "inst-1")
	broadcast := dial(t, p, "")
	waitFor(t, func() bool {
		p.clientsMux.RLock()
		defer p.clientsMux.RUnlock()
		return len(p.clients) == 1 && len(p.broadcast) == 1
	})

	// Fecha o socket do lado do servidor sem remover o cadastro.
	p.clientsMux.RLock()
	p.clients["inst-1"].conn.UnderlyingConn().Close()
	p.clientsMux.RUnlock()
	inst.Close()

	_ = p.Produce("message", []byte(`{}`), "inst-1", "") // pode devolver erro do cliente quebrado

	_ = broadcast.SetReadDeadline(time.Now().Add(3 * time.Second))
	var m map[string]interface{}
	if err := broadcast.ReadJSON(&m); err != nil {
		t.Fatalf("o broadcast não recebeu o evento: %v", err)
	}
}
