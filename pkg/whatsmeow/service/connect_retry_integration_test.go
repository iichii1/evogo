//go:build integration

package whatsmeow_service

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
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
	"go.mau.fi/whatsmeow/proto/waAdv"
	"go.mau.fi/whatsmeow/types"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestAPairedInstanceKeepsTryingWhenTheNetworkIsDownAndStopsOnRequest prova o que o teste unitário não alcança:
// o StartClient de uma instância já pareada, sem rede, tenta de novo com a espera combinada, ignora um segundo
// pedido de início enquanto isso e solta tudo na hora em que a sessão é parada. Exige uma rede sem saída
// (EVOGO_TEST_OFFLINE=1, container numa rede `docker network create --internal`): o aparelho aqui é inventado e
// nunca deve chegar ao WhatsApp.
func TestAPairedInstanceKeepsTryingWhenTheNetworkIsDownAndStopsOnRequest(t *testing.T) {
	authDSN := os.Getenv("EVOGO_TEST_POSTGRES_DSN")
	usersDSN := os.Getenv("EVOGO_TEST_POSTGRES_USERS_DSN")
	if authDSN == "" || usersDSN == "" || os.Getenv("EVOGO_TEST_OFFLINE") != "1" {
		t.Skip("EVOGO_TEST_POSTGRES_DSN, EVOGO_TEST_POSTGRES_USERS_DSN and EVOGO_TEST_OFFLINE=1 not set")
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

	// Versão fixa: sem isso o StartClient buscaria a versão na internet antes de conectar.
	cfg := &config.Config{PostgresAuthDB: authDSN, ClientName: "retry-test", LogDirectory: t.TempDir(),
		WhatsappVersionMajor: 2, WhatsappVersionMinor: 3000, WhatsappVersionPatch: 1}
	logger := logger_wrapper.NewLoggerManager(cfg)
	repo := instance_repository.NewInstanceRepository(db)
	clients := runtimestate.New[string, *whatsmeow.Client]()
	svc := NewWhatsmeowService(repo, authDB, message_repository.NewMessageRepository(db), label_repository.NewLabelRepository(db), cfg, clients, nil, nil, nil, nil, "", nil, nil, logger).(*whatsmeowService)

	container, err := svc.getStoreContainer()
	if err != nil {
		t.Fatal(err)
	}
	device := container.NewDevice()
	jid := types.JID{User: "999000000001", Device: 1, Server: types.DefaultUserServer}
	device.ID = &jid
	// O banco exige a identidade da conta; os valores só precisam existir, o aparelho nunca chega ao WhatsApp.
	device.Account = &waAdv.ADVSignedDeviceIdentity{Details: []byte{1}, AccountSignature: make([]byte, 64), AccountSignatureKey: make([]byte, 32), DeviceSignature: make([]byte, 64)}
	if err := device.Save(context.Background()); err != nil {
		t.Fatal(err)
	}

	const id = "7a1c2a9e-1b2c-4d5e-8f90-123456789b01"
	_ = repo.Delete(id)
	if _, err := repo.Create(instance_model.Instance{Id: id, Name: "retry-pareada", Token: "tok-retry", ClientName: "retry-test", Jid: jid.String(),
		Webhook: "http://destino.invalid/webhook", Events: "MESSAGE"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateConnected(id, true, ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Delete(id); svc.StopClient(id, "test finished") })

	logPath := filepath.Join(cfg.LogDirectory, id, "instance.log")
	logged := func(text string) bool {
		data, _ := os.ReadFile(logPath)
		return strings.Contains(string(data), text)
	}

	if err := svc.StartInstance(id); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "a primeira tentativa extra", 90*time.Second, func() bool { return logged("tentativa extra 1 de 8") })
	waitFor(t, "a segunda tentativa extra, que só vem depois de conectar de novo", 90*time.Second, func() bool { return logged("tentativa extra 2 de 8") })
	if !svc.connectingWithRetry(id) {
		t.Fatal("a instância deveria estar na fila de tentativas")
	}
	if c := clients.Get(id); c != nil && c.IsConnected() {
		t.Fatal("sem rede, não poderia estar conectada")
	}

	if err := svc.StartInstance(id); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "o segundo pedido de início ser ignorado", 10*time.Second, func() bool { return logged("Already retrying the connection") })

	started := time.Now()
	if !svc.StopClient(id, "test stop") {
		t.Fatal("StopClient deveria encontrar a sessão")
	}
	if took := time.Since(started); took > 5*time.Second {
		t.Errorf("parar durante a espera levou %s; deveria ser imediato", took)
	}
	waitFor(t, "a fila esvaziar", 5*time.Second, func() bool { return !svc.connectingWithRetry(id) && clients.Get(id) == nil })

	// A espera da tentativa 3 era de 10 s: se parar não a cancelasse, ela apareceria no log.
	time.Sleep(12 * time.Second)
	if logged("tentativa extra 3 de 8") {
		t.Error("a sessão foi parada, mas a fila continuou tentando")
	}
}
