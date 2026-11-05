package poll_handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	poll_model "github.com/evolution-foundation/evolution-go/pkg/poll/model"
	poll_service "github.com/evolution-foundation/evolution-go/pkg/poll/service"
)

type pollServiceStub struct {
	results *poll_model.PollResults
	err     error
}

func (s pollServiceStub) SavePollVote(context.Context, *poll_model.PollVote) error { return nil }
func (s pollServiceStub) GetPollResults(context.Context, string, string) (*poll_model.PollResults, error) {
	return s.results, s.err
}

func getResults(t *testing.T, svc poll_service.PollService) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	loggers := logger_wrapper.NewLoggerManager(&config.Config{LogDirectory: t.TempDir()})
	h := NewPollHandler(svc, loggers)

	r := gin.New()
	r.GET("/polls/:pollMessageId/results", func(c *gin.Context) {
		c.Set("instance", map[string]string{"id": "i1"})
	}, h.GetPollResults)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/polls/P1/results", nil))
	return w
}

func TestGetPollResultsWithoutPostgresIs501NotAMisleading404(t *testing.T) {
	w := getResults(t, pollServiceStub{err: poll_service.ErrPollStorageUnavailable})
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("status %d, esperado 501: %s", w.Code, w.Body)
	}
}

func TestGetPollResultsKeepsTheExistingStatuses(t *testing.T) {
	if w := getResults(t, pollServiceStub{results: &poll_model.PollResults{TotalVotes: 0}}); w.Code != http.StatusNotFound {
		t.Errorf("sem votos: status %d, esperado 404", w.Code)
	}
	if w := getResults(t, pollServiceStub{results: &poll_model.PollResults{PollMessageID: "P1", TotalVotes: 2}}); w.Code != http.StatusOK {
		t.Errorf("com votos: status %d, esperado 200", w.Code)
	}
	if w := getResults(t, pollServiceStub{err: context.DeadlineExceeded}); w.Code != http.StatusInternalServerError {
		t.Errorf("erro do banco: status %d, esperado 500", w.Code)
	}
}
