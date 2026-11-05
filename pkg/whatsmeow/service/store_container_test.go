package whatsmeow_service

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	_ "modernc.org/sqlite"
)

func TestGetStoreContainerReusesSQLiteContainer(t *testing.T) {
	exPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(exPath, "dbdata"), 0o755); err != nil {
		t.Fatal(err)
	}

	w := &whatsmeowService{
		config:         &config.Config{},
		exPath:         exPath,
		storeContainer: &storeContainerHolder{},
	}

	first, err := w.getStoreContainer()
	if err != nil {
		t.Fatalf("first getStoreContainer: %v", err)
	}
	for i := 0; i < 10; i++ {
		got, err := w.getStoreContainer()
		if err != nil {
			t.Fatalf("getStoreContainer #%d: %v", i, err)
		}
		if got != first {
			t.Fatalf("getStoreContainer #%d returned a new container; expected the shared one", i)
		}
	}
}

func TestGetStoreContainerSharedAcrossValueCopies(t *testing.T) {
	exPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(exPath, "dbdata"), 0o755); err != nil {
		t.Fatal(err)
	}

	original := whatsmeowService{
		config:         &config.Config{},
		exPath:         exPath,
		storeContainer: &storeContainerHolder{},
	}

	// StartClient usa receiver por valor, então cada chamada trabalha numa
	// cópia do serviço. As cópias precisam enxergar o mesmo container.
	copyA := original
	copyB := original

	a, err := copyA.getStoreContainer()
	if err != nil {
		t.Fatal(err)
	}
	b, err := copyB.getStoreContainer()
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("value copies of the service created different containers")
	}
}

func TestGetStoreContainerRetriesAfterError(t *testing.T) {
	// dbdata não existe, então o SQLite não consegue abrir main.db.
	exPath := filepath.Join(t.TempDir(), "missing")

	w := &whatsmeowService{
		config:         &config.Config{},
		exPath:         exPath,
		storeContainer: &storeContainerHolder{},
	}

	if _, err := w.getStoreContainer(); err == nil {
		t.Fatal("expected an error while dbdata does not exist")
	}

	if err := os.MkdirAll(filepath.Join(exPath, "dbdata"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := w.getStoreContainer(); err != nil {
		t.Fatalf("expected a retry to succeed once dbdata exists, got: %v", err)
	}
}

// TestGetStoreContainerPostgresDoesNotLeakConnections roda só quando
// EVOGO_TEST_POSTGRES_DSN aponta para um Postgres descartável, por exemplo:
//
//	EVOGO_TEST_POSTGRES_DSN="host=127.0.0.1 port=5432 user=postgres dbname=evogo_auth sslmode=disable"
func TestGetStoreContainerPostgresDoesNotLeakConnections(t *testing.T) {
	dsn := os.Getenv("EVOGO_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("EVOGO_TEST_POSTGRES_DSN not set")
	}

	authDB, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer authDB.Close()
	authDB.SetMaxOpenConns(5)
	authDB.SetMaxIdleConns(2)

	observer, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close()
	observer.SetMaxOpenConns(1)

	w := &whatsmeowService{
		config:         &config.Config{PostgresAuthDB: dsn},
		authDB:         authDB,
		storeContainer: &storeContainerHolder{},
	}

	// Simula 50 ciclos de StartClient de uma instância que reconecta em loop.
	for i := 0; i < 50; i++ {
		container, err := w.getStoreContainer()
		if err != nil {
			t.Fatalf("cycle %d: %v", i, err)
		}
		if _, err := container.GetAllDevices(context.Background()); err != nil {
			t.Fatalf("cycle %d: GetAllDevices: %v", i, err)
		}
	}

	var conns int
	err = observer.QueryRow(
		"SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND pid <> pg_backend_pid()",
	).Scan(&conns)
	if err != nil {
		t.Fatal(err)
	}

	// O pool authDB foi limitado a 5 conexões; antes da correção cada ciclo
	// abria um pool novo e este número crescia junto com os ciclos.
	if conns > 5 {
		t.Fatalf("expected at most 5 connections after 50 cycles, got %d", conns)
	}
	t.Logf("connections after 50 cycles: %d", conns)
}
