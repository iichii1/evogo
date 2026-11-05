package send_service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	"github.com/evolution-foundation/evolution-go/pkg/sendqueue"
)

// A fila vem das variáveis SEND_QUEUE_*: desligada por padrão, e ligada quando o
// intervalo mínimo é configurado.
func TestNewSendServiceBuildsTheQueueFromConfig(t *testing.T) {
	off := NewSendService(nil, nil, &config.Config{}, nil).(*sendService)
	for i := 0; i < 5; i++ {
		if waited, err := off.sendQueue.Acquire(context.Background(), "i1"); waited != 0 || err != nil {
			t.Fatalf("a fila deveria vir desligada, esperou %s / %v", waited, err)
		}
	}

	on := NewSendService(nil, nil, &config.Config{
		SendQueueMinInterval: time.Hour,
		SendQueueMaxWait:     time.Minute,
	}, nil).(*sendService)
	if _, err := on.sendQueue.Acquire(context.Background(), "i1"); err != nil {
		t.Fatal(err)
	}
	// A segunda só sairia em uma hora, mais que o máximo de espera configurado.
	if _, err := on.sendQueue.Acquire(context.Background(), "i1"); !errors.Is(err, sendqueue.ErrQueueTimeout) {
		t.Fatalf("erro = %v, esperado ErrQueueTimeout", err)
	}
}
