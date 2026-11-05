package message_service

import (
	"context"
	"sync"
	"time"

	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	"go.mau.fi/whatsmeow/types"
)

// presenceRestoreDelay é quanto esperar, depois do último "digitando", para
// voltar a ficar indisponível. Dá tempo de a mensagem seguinte sair enquanto
// o indicador ainda aparece para o contato.
const presenceRestoreDelay = 20 * time.Second

type presenceSender interface {
	SendPresence(ctx context.Context, state types.Presence) error
}

// presenceRestorer volta a instância para "indisponível" um tempo depois do
// último ChatPresence. Chamadas seguidas para a mesma instância reiniciam o
// prazo, então uma conversa com vários "digitando" só gera um envio no final.
type presenceRestorer struct {
	delay         time.Duration
	loggerWrapper *logger_wrapper.LoggerManager

	mu     sync.Mutex
	timers map[string]*time.Timer
}

func newPresenceRestorer(delay time.Duration, loggerWrapper *logger_wrapper.LoggerManager) *presenceRestorer {
	return &presenceRestorer{
		delay:         delay,
		loggerWrapper: loggerWrapper,
		timers:        make(map[string]*time.Timer),
	}
}

func (p *presenceRestorer) schedule(instanceID string, client presenceSender) {
	if p == nil || client == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if timer, ok := p.timers[instanceID]; ok {
		timer.Stop()
	}

	var timer *time.Timer
	timer = time.AfterFunc(p.delay, func() {
		p.mu.Lock()
		if p.timers[instanceID] == timer {
			delete(p.timers, instanceID)
		}
		p.mu.Unlock()

		if err := client.SendPresence(context.Background(), types.PresenceUnavailable); err != nil {
			if p.loggerWrapper != nil {
				p.loggerWrapper.GetLogger(instanceID).LogWarn("[%s] Failed to restore unavailable presence: %v", instanceID, err)
			}
			return
		}
		if p.loggerWrapper != nil {
			p.loggerWrapper.GetLogger(instanceID).LogInfo("[%s] Presence restored to unavailable (alwaysOnline=false)", instanceID)
		}
	})
	p.timers[instanceID] = timer
}
