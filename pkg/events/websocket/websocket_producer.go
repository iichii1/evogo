package websocket_producer

import (
	"net/http"
	"strings"
	"sync"
	"time"

	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	"github.com/gomessguii/logger"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		logger.LogInfo("Verificando origem da conexão WebSocket")
		return true
	},
}

// writeTimeout limita quanto um cliente lento pode segurar o envio dos eventos.
const writeTimeout = 10 * time.Second

// wsClient protege uma conexão contra escritas simultâneas. O gorilla/websocket
// permite um único escritor por conexão e dá panic ("concurrent write to
// websocket connection") quando duas goroutines escrevem ao mesmo tempo, o que
// derrubava o processo inteiro com vários eventos chegando juntos (#99).
type wsClient struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

func (c *wsClient) writeJSON(v interface{}) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	return c.conn.WriteJSON(v)
}

type websocketProducer struct {
	clients       map[string]*wsClient // conexões específicas por instância
	broadcast     []*wsClient          // conexões que recebem todos os eventos
	clientsMux    sync.RWMutex
	loggerWrapper *logger_wrapper.LoggerManager
}

func NewWebsocketProducer(loggerWrapper *logger_wrapper.LoggerManager) *websocketProducer {
	return &websocketProducer{
		clients:       make(map[string]*wsClient),
		broadcast:     make([]*wsClient, 0),
		clientsMux:    sync.RWMutex{},
		loggerWrapper: loggerWrapper,
	}
}

// ServeWs lida com as requisições de upgrade para websocket
func ServeWs(w http.ResponseWriter, r *http.Request, instanceId string, producer *websocketProducer) {
	logger.LogInfo("Iniciando upgrade da conexão WebSocket")
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.LogError("Erro ao fazer upgrade da conexão websocket: %v", err)
		return
	}

	logger.LogInfo("Conexão WebSocket estabelecida com sucesso")

	if instanceId == "" {
		producer.AddBroadcastClient(conn)
	} else {
		producer.AddClient(instanceId, conn)
	}

	// Goroutine para limpar conexão quando fechada
	go func() {
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				if instanceId == "" {
					producer.RemoveBroadcastClient(conn)
				} else {
					producer.removeClientIfCurrent(instanceId, conn)
				}
				conn.Close()
				break
			}
		}
	}()
}

func (p *websocketProducer) AddBroadcastClient(conn *websocket.Conn) {
	p.clientsMux.Lock()
	defer p.clientsMux.Unlock()
	p.broadcast = append(p.broadcast, &wsClient{conn: conn})
	logger.LogInfo("Cliente broadcast websocket adicionado")
}

func (p *websocketProducer) RemoveBroadcastClient(conn *websocket.Conn) {
	p.clientsMux.Lock()
	defer p.clientsMux.Unlock()
	for i, c := range p.broadcast {
		if c.conn == conn {
			// Copia em vez de alterar o array compartilhado com quem está enviando.
			next := make([]*wsClient, 0, len(p.broadcast)-1)
			next = append(next, p.broadcast[:i]...)
			next = append(next, p.broadcast[i+1:]...)
			p.broadcast = next
			break
		}
	}
	logger.LogInfo("Cliente broadcast websocket removido")
}

func (p *websocketProducer) AddClient(instanceID string, conn *websocket.Conn) {
	p.clientsMux.Lock()
	defer p.clientsMux.Unlock()
	p.clients[instanceID] = &wsClient{conn: conn}
	p.loggerWrapper.GetLogger(instanceID).LogInfo("Cliente websocket adicionado para instância: %s", instanceID)
}

func (p *websocketProducer) RemoveClient(instanceID string) {
	p.clientsMux.Lock()
	defer p.clientsMux.Unlock()
	delete(p.clients, instanceID)
	p.loggerWrapper.GetLogger(instanceID).LogInfo("Cliente websocket removido para instância: %s", instanceID)
}

// removeClientIfCurrent remove o cliente da instância só se ainda for esta
// conexão. Antes, a conexão antiga que caía removia a nova, que a tinha substituído.
func (p *websocketProducer) removeClientIfCurrent(instanceID string, conn *websocket.Conn) {
	p.clientsMux.Lock()
	defer p.clientsMux.Unlock()
	if current, ok := p.clients[instanceID]; ok && current.conn == conn {
		delete(p.clients, instanceID)
		p.loggerWrapper.GetLogger(instanceID).LogInfo("Cliente websocket removido para instância: %s", instanceID)
	}
}

func (p *websocketProducer) Produce(queueName string, payload []byte, instanceID string, _ string) error {
	message := map[string]interface{}{
		"queue":   strings.ToLower(queueName),
		"payload": string(payload),
	}

	// Copia os destinos sob o lock e escreve fora dele: um cliente lento não
	// trava mais o cadastro de novas conexões.
	p.clientsMux.RLock()
	client := p.clients[instanceID]
	broadcast := append([]*wsClient(nil), p.broadcast...)
	p.clientsMux.RUnlock()

	var firstErr error

	// Envia para cliente específico da instância
	if client != nil {
		if err := client.writeJSON(message); err != nil {
			p.loggerWrapper.GetLogger(instanceID).LogError("Erro ao enviar mensagem websocket para %s: %v", instanceID, err)
			firstErr = err
		} else {
			p.loggerWrapper.GetLogger(instanceID).LogInfo("Mensagem websocket enviada com sucesso para instância %s na fila %s", instanceID, queueName)
		}
	}

	// Envia para todos os clientes broadcast, mesmo se o da instância falhou
	for _, c := range broadcast {
		if err := c.writeJSON(message); err != nil {
			p.loggerWrapper.GetLogger(instanceID).LogError("Erro ao enviar mensagem broadcast websocket: %v", err)
		}
	}

	return firstErr
}

// CreateGlobalQueues não faz nada para websocket producer
func (p *websocketProducer) CreateGlobalQueues() error {
	return nil
}
