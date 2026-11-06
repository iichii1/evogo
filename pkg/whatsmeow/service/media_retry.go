package whatsmeow_service

import (
	"sync"

	"go.mau.fi/whatsmeow/types/events"
)

// mediaRetryWaiters liga o evento MediaRetry, que chega pela goroutine de eventos
// do whatsmeow, à requisição HTTP que está esperando por ele. A chave leva a
// instância: o mesmo ID de mensagem em outra instância nunca recebe esta resposta.
type mediaRetryWaiters struct {
	mu      sync.Mutex
	waiters map[string]map[chan *events.MediaRetry]struct{}
}

func newMediaRetryWaiters() *mediaRetryWaiters {
	return &mediaRetryWaiters{waiters: make(map[string]map[chan *events.MediaRetry]struct{})}
}

func mediaRetryKey(instanceID, messageID string) string {
	return instanceID + "/" + messageID
}

// wait registra um interessado e devolve o canal da resposta e a função que
// cancela o registro. Mais de uma requisição pode esperar a mesma mensagem.
func (w *mediaRetryWaiters) wait(instanceID, messageID string) (<-chan *events.MediaRetry, func()) {
	key := mediaRetryKey(instanceID, messageID)
	ch := make(chan *events.MediaRetry, 1)

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.waiters[key] == nil {
		w.waiters[key] = make(map[chan *events.MediaRetry]struct{})
	}
	w.waiters[key][ch] = struct{}{}

	return ch, func() {
		w.mu.Lock()
		defer w.mu.Unlock()
		delete(w.waiters[key], ch)
		if len(w.waiters[key]) == 0 {
			delete(w.waiters, key)
		}
	}
}

// deliver entrega a resposta a quem espera. O canal tem espaço para uma resposta,
// então a goroutine de eventos nunca fica presa aqui; quem já recebeu e ainda não
// cancelou o registro simplesmente não recebe uma segunda.
func (w *mediaRetryWaiters) deliver(instanceID string, evt *events.MediaRetry) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for ch := range w.waiters[mediaRetryKey(instanceID, evt.MessageID)] {
		select {
		case ch <- evt:
		default:
		}
	}
}

func (w *whatsmeowService) WaitMediaRetry(instanceId, messageId string) (<-chan *events.MediaRetry, func()) {
	return w.mediaRetries.wait(instanceId, messageId)
}

func (w *whatsmeowService) DeliverMediaRetry(instanceId string, evt *events.MediaRetry) {
	w.mediaRetries.deliver(instanceId, evt)
}
