package runtimestate

import (
	"sync"
	"time"
)

// Session representa uma "geração" de cliente de uma instância: do momento em
// que o StartClient cria o cliente até a limpeza terminar.
//
// Substitui o antigo chan bool por instância. Aquele canal tinha três
// problemas: o envio não-bloqueante se perdia quando o loop estava dormindo,
// mais de uma goroutine lia o mesmo canal e "roubava" o sinal, e fechar o canal
// fazia o loop reiniciar o cliente de uma instância apagada.
//
// Aqui parar é idempotente (pode ser chamado de qualquer lugar, quantas vezes
// for) e quem pediu a parada pode esperar a limpeza terminar.
type Session struct {
	stopOnce sync.Once
	doneOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}

	mu     sync.Mutex
	reason string
}

// NewSession cria uma sessão ativa.
func NewSession() *Session {
	return &Session{
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

// Stop pede que a sessão pare. Só a primeira chamada grava o motivo.
func (s *Session) Stop(reason string) {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		s.mu.Lock()
		s.reason = reason
		s.mu.Unlock()
		close(s.stopCh)
	})
}

// Stopping devolve um canal fechado quando a parada foi pedida.
func (s *Session) Stopping() <-chan struct{} {
	if s == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return s.stopCh
}

// IsStopping informa se a parada já foi pedida.
func (s *Session) IsStopping() bool {
	select {
	case <-s.Stopping():
		return true
	default:
		return false
	}
}

// Reason devolve o motivo passado ao primeiro Stop.
func (s *Session) Reason() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reason
}

// Finish marca que a limpeza da sessão terminou.
func (s *Session) Finish() {
	if s == nil {
		return
	}
	s.doneOnce.Do(func() { close(s.doneCh) })
}

// Done devolve um canal fechado quando a limpeza termina.
func (s *Session) Done() <-chan struct{} {
	if s == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return s.doneCh
}

// StopAndWait pede a parada e espera a limpeza até o timeout. Devolve false se
// o timeout estourou.
func (s *Session) StopAndWait(reason string, timeout time.Duration) bool {
	if s == nil {
		return true
	}
	s.Stop(reason)
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-s.Done():
		return true
	case <-timer.C:
		return false
	}
}
