package sendqueue

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/evolution-foundation/evolution-go/pkg/metrics"
)

// newTestLimiter usa um relógio e um sorteio controlados: o horário só anda
// quando o teste manda, e o "sorteio" devolve sempre o valor escolhido.
func newTestLimiter(cfg Config, draw int64) (*Limiter, *time.Time) {
	clock := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	l := New(cfg)
	l.now = func() time.Time { return clock }
	l.jitter = func(n int64) int64 {
		if draw >= n {
			return n - 1
		}
		return draw
	}
	return l, &clock
}

func TestDisabledLimiterNeverWaits(t *testing.T) {
	for _, l := range []*Limiter{nil, New(Config{}), New(Config{MaxInterval: time.Minute, MaxPending: 1})} {
		for i := 0; i < 50; i++ {
			if waited, err := l.Acquire(context.Background(), "i1"); waited != 0 || err != nil {
				t.Fatalf("limitador desligado esperou %s / %v", waited, err)
			}
		}
	}
	if New(Config{}).cfg.Enabled() {
		t.Error("sem intervalo mínimo o limitador está desligado")
	}
}

func TestIntervalIsDrawnBetweenMinAndMax(t *testing.T) {
	l, _ := newTestLimiter(Config{MinInterval: 2 * time.Second, MaxInterval: 5 * time.Second}, 0)
	if got := l.interval(); got != 2*time.Second {
		t.Errorf("sorteio mínimo: %s", got)
	}

	l, _ = newTestLimiter(Config{MinInterval: 2 * time.Second, MaxInterval: 5 * time.Second}, 1<<40)
	if got := l.interval(); got != 5*time.Second {
		t.Errorf("sorteio máximo: %s", got)
	}

	// Máximo menor que o mínimo vira intervalo fixo.
	l, _ = newTestLimiter(Config{MinInterval: 3 * time.Second, MaxInterval: time.Second}, 7)
	if got := l.interval(); got != 3*time.Second {
		t.Errorf("máximo menor que o mínimo: %s", got)
	}

	// Com o sorteio de verdade, sempre dentro da faixa.
	real := New(Config{MinInterval: 2 * time.Second, MaxInterval: 5 * time.Second})
	for i := 0; i < 200; i++ {
		if got := real.interval(); got < 2*time.Second || got > 5*time.Second {
			t.Fatalf("intervalo fora da faixa: %s", got)
		}
	}
}

// As reservas são em sequência: a 1ª sai na hora, a 2ª espera 1 intervalo, a 3ª 2.
func TestSlotsAreReservedInOrder(t *testing.T) {
	l, _ := newTestLimiter(Config{MinInterval: 40 * time.Millisecond}, 0)

	// O relógio do teste não anda: a espera de cada uma é a distância até o seu
	// horário reservado (0, 1 e 2 intervalos).
	var got []time.Duration
	for i := 0; i < 3; i++ {
		waited, err := l.Acquire(context.Background(), "i1")
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, waited)
	}

	if got[0] != 0 || got[1] != 40*time.Millisecond || got[2] != 80*time.Millisecond {
		t.Fatalf("esperas = %v, esperado [0 40ms 80ms]", got)
	}
}

func TestInstancesDoNotWaitForEachOther(t *testing.T) {
	l, _ := newTestLimiter(Config{MinInterval: time.Hour, MaxWait: time.Millisecond}, 0)

	if _, err := l.Acquire(context.Background(), "A"); err != nil {
		t.Fatal(err)
	}
	// A instância B tem a fila dela: sai na hora, sem herdar a espera de A.
	if waited, err := l.Acquire(context.Background(), "B"); err != nil || waited != 0 {
		t.Fatalf("B esperou %s / %v", waited, err)
	}
}

func TestFullQueueRejectsWithRetryAfter(t *testing.T) {
	l, _ := newTestLimiter(Config{MinInterval: 200 * time.Millisecond, MaxPending: 1}, 0)
	fullBefore := metrics.SendQueueRejectedFull.Value()

	if _, err := l.Acquire(context.Background(), "i1"); err != nil { // sai na hora
		t.Fatal(err)
	}

	// A 2ª fica esperando (ocupa a única vaga da fila)...
	secondDone := make(chan error, 1)
	go func() {
		_, err := l.Acquire(context.Background(), "i1")
		secondDone <- err
	}()
	waitFor(t, func() bool { return waitingOf(l, "i1") == 1 })

	// ...então a 3ª é recusada na hora, dizendo quando tentar de novo.
	_, err := l.Acquire(context.Background(), "i1")
	var rejected *RejectedError
	if !errors.As(err, &rejected) || !errors.Is(err, ErrQueueFull) {
		t.Fatalf("erro = %v, esperado fila cheia", err)
	}
	if rejected.RetryAfter <= 0 {
		t.Errorf("RetryAfter = %s", rejected.RetryAfter)
	}
	if err := <-secondDone; err != nil {
		t.Errorf("a 2ª mensagem deveria sair: %v", err)
	}
	if got := metrics.SendQueueRejectedFull.Value() - fullBefore; got != 1 {
		t.Errorf("contador de recusadas por fila cheia subiu %d, esperado 1", got)
	}
}

func TestWaitLongerThanMaxWaitIsRejected(t *testing.T) {
	l, _ := newTestLimiter(Config{MinInterval: time.Hour, MaxWait: time.Minute}, 0)
	timeoutBefore := metrics.SendQueueRejectedTimeout.Value()

	if _, err := l.Acquire(context.Background(), "i1"); err != nil {
		t.Fatal(err)
	}
	// A próxima só sairia em 1 hora, mais que o máximo de 1 minuto: recusa na hora,
	// em vez de segurar a requisição até o cliente desistir.
	started := time.Now()
	_, err := l.Acquire(context.Background(), "i1")
	if !errors.Is(err, ErrQueueTimeout) {
		t.Fatalf("erro = %v, esperado ErrQueueTimeout", err)
	}
	if time.Since(started) > time.Second {
		t.Error("a recusa deveria ser imediata")
	}
	var rejected *RejectedError
	if errors.As(err, &rejected) && rejected.RetryAfter < 58*time.Minute {
		t.Errorf("RetryAfter = %s, esperado cerca de 59 minutos", rejected.RetryAfter)
	}
	if got := metrics.SendQueueRejectedTimeout.Value() - timeoutBefore; got != 1 {
		t.Errorf("contador de recusadas por tempo subiu %d, esperado 1", got)
	}
}

func TestCancelledContextStopsWaitingAndFreesTheSeat(t *testing.T) {
	l, _ := newTestLimiter(Config{MinInterval: time.Hour, MaxPending: 1}, 0)
	if _, err := l.Acquire(context.Background(), "i1"); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := l.Acquire(ctx, "i1")
		done <- err
	}()
	waitFor(t, func() bool { return waitingOf(l, "i1") == 1 })

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("erro = %v, esperado context.Canceled", err)
	}
	if got := waitingOf(l, "i1"); got != 0 {
		t.Errorf("a vaga da fila não foi liberada: %d esperando", got)
	}
}

func TestConcurrentSendersAreSerializedPerInstance(t *testing.T) {
	l := New(Config{MinInterval: 15 * time.Millisecond})

	const senders = 6
	var mu sync.Mutex
	var starts []time.Time
	var wg sync.WaitGroup
	for i := 0; i < senders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := l.Acquire(context.Background(), "i1"); err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			starts = append(starts, time.Now())
			mu.Unlock()
		}()
	}
	wg.Wait()

	if len(starts) != senders {
		t.Fatalf("saíram %d de %d", len(starts), senders)
	}
	first, last := starts[0], starts[0]
	for _, s := range starts {
		if s.Before(first) {
			first = s
		}
		if s.After(last) {
			last = s
		}
	}
	// 6 envios com 15 ms entre eles levam no mínimo 5 intervalos (75 ms), com folga
	// para o agendador do sistema.
	if span := last.Sub(first); span < 60*time.Millisecond {
		t.Fatalf("os envios saíram juntos demais: %s entre o primeiro e o último", span)
	}
}

func waitingOf(l *Limiter, id string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if ln := l.lanes[id]; ln != nil {
		return ln.waiting
	}
	return 0
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condição não ficou verdadeira a tempo")
		}
		time.Sleep(time.Millisecond)
	}
}
