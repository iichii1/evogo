// Package sendqueue espaça os envios de mensagens de cada instância.
//
// O erro 463 e as contas restritas depois de 25 a 50 mensagens seguidas (#236,
// #50) mostram o risco de disparar rápido demais por uma conta de WhatsApp. O
// servidor não tinha nenhum freio: quem usa a API em lote tinha de se controlar
// do lado de fora.
//
// O limitador reserva horários, um por mensagem e por instância, separados por
// um intervalo aleatório entre o mínimo e o máximo configurados (um intervalo
// fixo é fácil de reconhecer como automação). Quem chega antes envia antes.
// Instâncias diferentes não se atrapalham: cada uma tem a sua fila.
//
// Fica desligado enquanto o intervalo mínimo for zero, que é o padrão: ligar é
// decisão de quem opera o servidor (SEND_QUEUE_MIN_INTERVAL_MS).
package sendqueue

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/evolution-foundation/evolution-go/pkg/metrics"
)

var (
	// ErrQueueFull indica que já há mensagens demais esperando para sair pela
	// instância.
	ErrQueueFull = errors.New("send queue is full for this instance")
	// ErrQueueTimeout indica que a vez da mensagem só chegaria depois do tempo
	// máximo de espera.
	ErrQueueTimeout = errors.New("send queue wait exceeds the maximum")
)

// RejectedError é o erro devolvido quando a mensagem não entra na fila. Reason é
// ErrQueueFull ou ErrQueueTimeout; RetryAfter diz quando vale tentar de novo.
type RejectedError struct {
	Reason     error
	RetryAfter time.Duration
}

func (e *RejectedError) Error() string {
	return fmt.Sprintf("%v; try again in %s", e.Reason, e.RetryAfter.Round(time.Second))
}

func (e *RejectedError) Unwrap() error { return e.Reason }

// Config define o ritmo de envio de cada instância.
type Config struct {
	// MinInterval é o menor intervalo entre duas mensagens da mesma instância.
	// Zero desliga o limitador.
	MinInterval time.Duration
	// MaxInterval é o maior intervalo. O de cada mensagem é sorteado entre
	// MinInterval e MaxInterval. Se for menor que MinInterval, usa MinInterval.
	MaxInterval time.Duration
	// MaxPending é quantas mensagens podem esperar ao mesmo tempo por instância.
	// Zero significa sem limite.
	MaxPending int
	// MaxWait é o quanto uma mensagem espera, no máximo, pela sua vez. Zero
	// significa sem limite. Deve ficar abaixo do tempo limite de quem chama a API.
	MaxWait time.Duration
}

// Enabled informa se o limitador está ligado.
func (c Config) Enabled() bool { return c.MinInterval > 0 }

type lane struct {
	// next é o horário mais cedo em que a próxima mensagem pode sair.
	next    time.Time
	waiting int
}

// Limiter controla o ritmo de envio. O valor zero e o ponteiro nulo não limitam.
type Limiter struct {
	cfg Config
	now func() time.Time
	// jitter devolve um número em [0, n). Trocado nos testes.
	jitter func(n int64) int64

	mu    sync.Mutex
	lanes map[string]*lane
}

// New cria o limitador. Com MinInterval zero ele não faz nada.
func New(cfg Config) *Limiter {
	if cfg.MaxInterval < cfg.MinInterval {
		cfg.MaxInterval = cfg.MinInterval
	}
	return &Limiter{
		cfg:    cfg,
		now:    time.Now,
		jitter: rand.Int64N,
		lanes:  map[string]*lane{},
	}
}

func (l *Limiter) interval() time.Duration {
	spread := int64(l.cfg.MaxInterval - l.cfg.MinInterval)
	if spread <= 0 {
		return l.cfg.MinInterval
	}
	return l.cfg.MinInterval + time.Duration(l.jitter(spread+1))
}

// Acquire espera a vez da instância e devolve quanto esperou. Devolve
// *RejectedError (ErrQueueFull ou ErrQueueTimeout) se a mensagem não cabe na
// fila, ou o erro do contexto se ele terminar antes da vez.
//
// A vez é reservada na hora: quem chega primeiro sai primeiro, e o horário de
// uma mensagem que desiste (contexto cancelado) não é reaproveitado, o que só
// deixa o ritmo mais conservador.
func (l *Limiter) Acquire(ctx context.Context, instanceID string) (time.Duration, error) {
	if l == nil || !l.cfg.Enabled() {
		return 0, nil
	}

	l.mu.Lock()
	ln := l.lanes[instanceID]
	if ln == nil {
		ln = &lane{}
		l.lanes[instanceID] = ln
	}

	now := l.now()
	start := now
	if ln.next.After(now) {
		start = ln.next
	}
	wait := start.Sub(now)

	if wait > 0 {
		if l.cfg.MaxPending > 0 && ln.waiting >= l.cfg.MaxPending {
			l.mu.Unlock()
			metrics.SendQueueRejectedFull.Inc()
			return 0, &RejectedError{Reason: ErrQueueFull, RetryAfter: wait}
		}
		if l.cfg.MaxWait > 0 && wait > l.cfg.MaxWait {
			l.mu.Unlock()
			metrics.SendQueueRejectedTimeout.Inc()
			return 0, &RejectedError{Reason: ErrQueueTimeout, RetryAfter: wait - l.cfg.MaxWait}
		}
		ln.waiting++
		metrics.SendQueueWaiting.Add(1)
	}
	ln.next = start.Add(l.interval())
	l.mu.Unlock()

	if wait <= 0 {
		return 0, nil
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()

	var err error
	select {
	case <-timer.C:
	case <-ctx.Done():
		err = ctx.Err()
	}

	l.mu.Lock()
	ln.waiting--
	l.mu.Unlock()
	metrics.SendQueueWaiting.Add(-1)

	if err != nil {
		return 0, err
	}
	metrics.SendQueueDelayed.Inc()
	metrics.SendQueueWaitMilliseconds.Add(uint64(wait.Milliseconds()))
	return wait, nil
}
