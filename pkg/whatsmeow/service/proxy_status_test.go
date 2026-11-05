package whatsmeow_service

import (
	"testing"
	"time"

	"github.com/evolution-foundation/evolution-go/pkg/runtimestate"
)

func newProxyStatusService() whatsmeowService {
	return whatsmeowService{proxyStatus: runtimestate.New[string, *ProxyRuntimeStatus]()}
}

func TestProxyRuntimeStatusUnknownUntilTheClientStarts(t *testing.T) {
	w := newProxyStatusService()
	if _, started := w.ProxyRuntimeStatus("i1"); started {
		t.Fatal("instância que nunca iniciou não tem estado de proxy")
	}
}

func TestMarkProxyFallbackKeepsWhenItWasApplied(t *testing.T) {
	w := newProxyStatusService()
	appliedAt := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	w.setProxyStatus("i1", ProxyRuntimeStatus{RuntimeEnabled: true, LastAppliedAt: appliedAt})

	w.markProxyFallback("i1")

	got, started := w.ProxyRuntimeStatus("i1")
	if !started {
		t.Fatal("o estado deveria existir")
	}
	if got.RuntimeEnabled || !got.FallbackWithoutProxy || got.LastError == "" {
		t.Errorf("depois do fallback: %+v", got)
	}
	if !got.LastAppliedAt.Equal(appliedAt) {
		t.Errorf("lastAppliedAt perdido: %v", got.LastAppliedAt)
	}
}

// Serviço montado sem o mapa (como nos testes de ciclo de vida) não pode dar panic.
func TestProxyStatusWithoutTheMapDoesNotPanic(t *testing.T) {
	w := whatsmeowService{}
	w.setProxyStatus("i1", ProxyRuntimeStatus{RuntimeEnabled: true})
	w.markProxyFallback("i1")
	if _, started := w.ProxyRuntimeStatus("i1"); started {
		t.Fatal("sem mapa não há estado")
	}
}
