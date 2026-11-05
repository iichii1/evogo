//go:build integration

package whatsmeow_service

import (
	"database/sql"
	"os"
	"runtime"
	"sync"
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

func waitFor(t *testing.T, what string, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// TestLifecycleIntegration conecta de verdade aos servidores do WhatsApp com um
// aparelho novo (só gera QR, nada é pareado) e confere o ciclo de vida:
// start, starts concorrentes, reconexão, parada e exclusão.
//
// Rode com:
//
//	EVOGO_TEST_POSTGRES_DSN="host=127.0.0.1 user=postgres dbname=evogo_auth sslmode=disable" \
//	EVOGO_TEST_POSTGRES_USERS_DSN="host=127.0.0.1 user=postgres dbname=evogo_users sslmode=disable" \
//	go test -race -tags integration ./pkg/whatsmeow/service/ -run TestLifecycleIntegration -v
func TestLifecycleIntegration(t *testing.T) {
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

	cfg := &config.Config{PostgresAuthDB: authDSN, ClientName: "test", LogDirectory: t.TempDir()}
	logger := logger_wrapper.NewLoggerManager(cfg)
	repo := instance_repository.NewInstanceRepository(db)
	clients := runtimestate.New[string, *whatsmeow.Client]()

	svcIface := NewWhatsmeowService(repo, authDB, message_repository.NewMessageRepository(db), label_repository.NewLabelRepository(db), cfg, clients, nil, nil, nil, nil, "", nil, nil, logger)
	svc := svcIface.(*whatsmeowService)

	// Remove sobras de uma execução anterior.
	_ = repo.Delete("6f1c2a9e-1b2c-4d5e-8f90-123456789abc")

	inst, err := repo.Create(instance_model.Instance{Id: "6f1c2a9e-1b2c-4d5e-8f90-123456789abc", Name: "itest-1", Token: "tok-itest-1", ClientName: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repo.Delete(inst.Id) })

	baseGoroutines := runtime.NumGoroutine()

	// 1. Start: new device connects and gets QR codes.
	go svc.StartClient(&ClientData{Instance: inst})
	waitFor(t, "client connected (pairing)", 30*time.Second, func() bool {
		c := clients.Get(inst.Id)
		return c != nil && c.IsConnected()
	})
	waitFor(t, "QR code stored", 30*time.Second, func() bool {
		i, err := repo.GetInstanceByID(inst.Id)
		return err == nil && i.Qrcode != ""
	})
	first := clients.Get(inst.Id)
	t.Logf("step1 ok: connected, QR stored")

	// 2. Concurrent starts while running must not create a second client.
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); svc.StartClient(&ClientData{Instance: inst}) }()
	}
	wg.Wait()
	if clients.Get(inst.Id) != first {
		t.Fatal("concurrent StartClient replaced a healthy client")
	}
	if svc.sessions.Len() != 1 {
		t.Fatalf("sessions = %d, want 1", svc.sessions.Len())
	}
	t.Logf("step2 ok: concurrent starts were no-ops")

	// 3. Reconnect replaces the client and the old one is fully gone.
	oldSession := svc.sessions.Get(inst.Id)
	if err := svc.ReconnectClient(inst.Id); err != nil {
		t.Fatal(err)
	}
	select {
	case <-oldSession.Done():
	default:
		t.Fatal("old session not finished after ReconnectClient")
	}
	waitFor(t, "new client connected", 30*time.Second, func() bool {
		c := clients.Get(inst.Id)
		return c != nil && c != first && c.IsConnected()
	})
	if first.IsConnected() {
		t.Fatal("old client still connected after reconnect")
	}
	if svc.sessions.Len() != 1 {
		t.Fatalf("sessions after reconnect = %d, want 1", svc.sessions.Len())
	}
	t.Logf("step3 ok: reconnect replaced client")

	// 4. Stop via API: client removed and NOT restarted.
	second := clients.Get(inst.Id)
	if !svc.StopClient(inst.Id, stopReasonAPI) {
		t.Fatal("StopClient found nothing running")
	}
	if clients.Get(inst.Id) != nil || svc.sessions.Len() != 0 || svc.myClientPointer.Len() != 0 {
		t.Fatalf("registries not clean: client=%v sessions=%d mycli=%d", clients.Get(inst.Id) != nil, svc.sessions.Len(), svc.myClientPointer.Len())
	}
	if second.IsConnected() {
		t.Fatal("client still connected after StopClient")
	}
	time.Sleep(5 * time.Second)
	if clients.Get(inst.Id) != nil {
		t.Fatal("client was restarted after an API stop")
	}
	i, _ := repo.GetInstanceByID(inst.Id)
	if i.Connected || i.DisconnectReason != stopReasonAPI {
		t.Fatalf("db state connected=%v reason=%q", i.Connected, i.DisconnectReason)
	}
	t.Logf("step4 ok: stopped, not restarted, db=%q", i.DisconnectReason)

	// 5. Start + delete (ClearInstanceCache) while pairing.
	go svc.StartClient(&ClientData{Instance: inst})
	waitFor(t, "client connected again", 30*time.Second, func() bool {
		c := clients.Get(inst.Id)
		return c != nil && c.IsConnected()
	})
	third := clients.Get(inst.Id)
	if err := svc.ClearInstanceCache(inst.Id, inst.Token); err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * time.Second)
	if clients.Get(inst.Id) != nil || third.IsConnected() {
		t.Fatal("client survived ClearInstanceCache")
	}
	t.Logf("step5 ok: delete stopped client")

	time.Sleep(2 * time.Second)
	t.Logf("goroutines: base=%d now=%d", baseGoroutines, runtime.NumGoroutine())

	var conns int
	authDB.QueryRow("SELECT count(*) FROM pg_stat_activity WHERE datname='evogo_auth'").Scan(&conns)
	t.Logf("auth db connections after 3 client generations: %d", conns)
}
