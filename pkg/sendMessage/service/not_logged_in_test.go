package send_service

import (
	"errors"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	"github.com/evolution-foundation/evolution-go/pkg/runtimestate"
)

// newUnpairedService devolve um serviço cuja instância "i1" tem um cliente sem
// conta pareada (Store.ID nulo) e SEM whatsmeowService: se o código tentar
// reconectar, o teste dá panic em vez de esperar em silêncio.
func newUnpairedService(t *testing.T) *sendService {
	t.Helper()
	clients := runtimestate.New[string, *whatsmeow.Client]()
	clients.Set("i1", whatsmeow.NewClient(&store.Device{}, nil))
	return &sendService{
		clientPointer: clients,
		config:        &config.Config{},
		loggerWrapper: logger_wrapper.NewLoggerManager(&config.Config{LogDirectory: t.TempDir()}),
	}
}

// Regressão de #77: em instância sem conta pareada o /send/text demorava mais de
// um minuto repetindo reconexões antes de devolver o erro.
func TestEnsureClientConnectedFailsFastWhenNotLoggedIn(t *testing.T) {
	s := newUnpairedService(t)

	start := time.Now()
	_, err := s.ensureClientConnectedWithRetry("i1", 2)

	if !errors.Is(err, ErrInstanceNotLoggedIn) {
		t.Fatalf("err = %v, want ErrInstanceNotLoggedIn", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("levou %v, deveria falhar na hora", elapsed)
	}
}

func TestSendTextFailsFastWhenNotLoggedIn(t *testing.T) {
	s := newUnpairedService(t)

	start := time.Now()
	_, err := s.SendText(&TextStruct{Number: "5511999999999", Text: "oi"}, &instance_model.Instance{Id: "i1"})

	if !errors.Is(err, ErrInstanceNotLoggedIn) {
		t.Fatalf("err = %v, want ErrInstanceNotLoggedIn", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("levou %v, deveria falhar na hora (antes passava de um minuto)", elapsed)
	}
}

func TestIsUnpaired(t *testing.T) {
	if isUnpaired(nil) {
		t.Fatal("cliente nulo não é 'sem conta pareada': o fluxo normal inicia a instância")
	}
	if !isUnpaired(whatsmeow.NewClient(&store.Device{}, nil)) {
		t.Fatal("cliente sem Store.ID deve contar como sem conta pareada")
	}
}
