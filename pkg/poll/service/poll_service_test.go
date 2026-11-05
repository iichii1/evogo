package poll_service

import (
	"context"
	"errors"
	"testing"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	"github.com/evolution-foundation/evolution-go/pkg/poll/model"
)

// Sem POSTGRES_AUTH_DB (modo SQLite) o banco é nulo, e NewPollService derrubava o
// servidor na inicialização com um ponteiro nulo.
func TestNewPollServiceWithoutDatabaseDoesNotPanic(t *testing.T) {
	loggers := logger_wrapper.NewLoggerManager(&config.Config{LogDirectory: t.TempDir()})

	svc := NewPollService(nil, loggers)

	if err := svc.SavePollVote(context.Background(), &model.PollVote{PollMessageID: "P1"}); !errors.Is(err, ErrPollStorageUnavailable) {
		t.Errorf("SavePollVote sem banco: erro = %v, esperado ErrPollStorageUnavailable", err)
	}
	if _, err := svc.GetPollResults(context.Background(), "P1", "i1"); !errors.Is(err, ErrPollStorageUnavailable) {
		t.Errorf("GetPollResults sem banco: erro = %v, esperado ErrPollStorageUnavailable", err)
	}
}
