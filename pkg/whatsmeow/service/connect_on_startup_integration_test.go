//go:build integration

package whatsmeow_service

import (
	"database/sql"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	instance_repository "github.com/evolution-foundation/evolution-go/pkg/instance/repository"
	label_model "github.com/evolution-foundation/evolution-go/pkg/label/model"
	label_repository "github.com/evolution-foundation/evolution-go/pkg/label/repository"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	message_model "github.com/evolution-foundation/evolution-go/pkg/message/model"
	message_repository "github.com/evolution-foundation/evolution-go/pkg/message/repository"
	"github.com/evolution-foundation/evolution-go/pkg/runtimestate"
	"go.mau.fi/whatsmeow"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestConnectOnStartupBringsBackWhatWasConnected faz o que um reinício do processo faz: estado em memória
// vazio, mesmo banco, CONNECT_ON_STARTUP ligado. Só volta o que estava conectado e é deste CLIENT_NAME, com
// o webhook e as assinaturas salvos. Sem esse caminho, a instância fica parada até alguém chamar
// /instance/connect (ensaio de 2026-11-06). Como TestLifecycleIntegration, conecta de verdade com aparelhos
// novos, que só geram QR; o ramo do aparelho já pareado foi provado com as contas de teste.
func TestConnectOnStartupBringsBackWhatWasConnected(t *testing.T) {
	authDSN := os.Getenv("EVOGO_TEST_POSTGRES_DSN")
	usersDSN := os.Getenv("EVOGO_TEST_POSTGRES_USERS_DSN")
	if authDSN == "" || usersDSN == "" {
		t.Skip("EVOGO_TEST_POSTGRES_DSN and EVOGO_TEST_POSTGRES_USERS_DSN not set")
	}

	db, err := gorm.Open(postgres.Open(usersDSN), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&instance_model.Instance{}, &label_model.Label{}, &message_model.Message{}); err != nil {
		t.Fatal(err)
	}
	authDB, err := sql.Open("postgres", authDSN)
	if err != nil {
		t.Fatal(err)
	}
	authDB.SetMaxOpenConns(5)

	const clientName = "startup-test"
	repo := instance_repository.NewInstanceRepository(db)
	create := func(id, name, client string, connected bool) *instance_model.Instance {
		t.Helper()
		_ = repo.Delete(id)
		inst, err := repo.Create(instance_model.Instance{
			Id: id, Name: name, Token: "tok-" + name, ClientName: client,
			Webhook: "http://destino.invalid/webhook", Events: "MESSAGE,GROUP,CHAT_SYNC",
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := repo.UpdateConnected(id, connected, ""); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = repo.Delete(id) })
		return inst
	}
	wanted := create("7a1c2a9e-1b2c-4d5e-8f90-123456789a01", "startup-conectada", clientName, true)
	stopped := create("7a1c2a9e-1b2c-4d5e-8f90-123456789a02", "startup-parada", clientName, false)
	foreign := create("7a1c2a9e-1b2c-4d5e-8f90-123456789a03", "startup-de-outro", "outro-cliente", true)

	cfg := &config.Config{PostgresAuthDB: authDSN, ClientName: clientName, LogDirectory: t.TempDir()}
	logger := logger_wrapper.NewLoggerManager(cfg)
	clients := runtimestate.New[string, *whatsmeow.Client]()
	svc := NewWhatsmeowService(repo, authDB, message_repository.NewMessageRepository(db), label_repository.NewLabelRepository(db), cfg, clients, nil, nil, nil, nil, "", nil, nil, logger).(*whatsmeowService)

	svc.ConnectOnStartup(clientName)
	t.Cleanup(func() { svc.StopClient(wanted.Id, "test finished") })

	waitFor(t, "a instância conectada voltar", 30*time.Second, func() bool {
		c := clients.Get(wanted.Id)
		return c != nil && c.IsConnected()
	})

	mycli := svc.myClientPointer.Get(wanted.Id)
	if mycli == nil {
		t.Fatal("cliente sem registro")
	}
	if mycli.webhookUrl != wanted.Webhook {
		t.Errorf("o webhook salvo não voltou: %q", mycli.webhookUrl)
	}
	for _, event := range []string{"MESSAGE", "GROUP", "CHAT_SYNC"} {
		if !slices.Contains(mycli.subscriptions, event) {
			t.Errorf("a assinatura %s não voltou: %v", event, mycli.subscriptions)
		}
	}

	time.Sleep(3 * time.Second)
	if clients.Get(stopped.Id) != nil {
		t.Error("a instância parada de propósito não deveria voltar")
	}
	if clients.Get(foreign.Id) != nil {
		t.Error("a instância de outro CLIENT_NAME não deveria voltar")
	}
}
