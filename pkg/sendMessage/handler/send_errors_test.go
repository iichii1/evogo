package send_handler

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/evolution-foundation/evolution-go/pkg/sendqueue"
)

func respond(err error) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", func(c *gin.Context) { respondSendError(c, err) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	return w
}

func TestRespondSendErrorTurnsAFullQueueInto429WithRetryAfter(t *testing.T) {
	err := fmt.Errorf("envio: %w", &sendqueue.RejectedError{Reason: sendqueue.ErrQueueFull, RetryAfter: 12400 * time.Millisecond})

	w := respond(err)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status %d, esperado 429: %s", w.Code, w.Body)
	}
	if got := w.Header().Get("Retry-After"); got != "13" {
		t.Errorf("Retry-After = %q, esperado 13 (arredondado para cima)", got)
	}
	if !strings.Contains(w.Body.String(), `"retryAfter":13`) {
		t.Errorf("corpo = %s", w.Body)
	}
}

func TestRespondSendErrorRetryAfterIsAtLeastOneSecond(t *testing.T) {
	w := respond(&sendqueue.RejectedError{Reason: sendqueue.ErrQueueTimeout, RetryAfter: 10 * time.Millisecond})
	if got := w.Header().Get("Retry-After"); got != "1" {
		t.Errorf("Retry-After = %q, esperado 1", got)
	}
}

// O que não é da fila segue como antes: 500 com o texto do erro.
func TestRespondSendErrorKeepsTheOtherErrorsAs500(t *testing.T) {
	w := respond(errors.New("instance is not logged in"))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, esperado 500", w.Code)
	}
	if w.Header().Get("Retry-After") != "" {
		t.Error("Retry-After só vale para a fila")
	}
	if !strings.Contains(w.Body.String(), "instance is not logged in") {
		t.Errorf("corpo = %s", w.Body)
	}
}

func TestParseExpirationForm(t *testing.T) {
	if got, err := parseExpirationForm(""); got != nil || err != nil {
		t.Fatalf("vazio: %v, %v", got, err)
	}
	got, err := parseExpirationForm("604800")
	if err != nil || got == nil || *got != 604800 {
		t.Fatalf("número: %v, %v", got, err)
	}
	for _, bad := range []string{"-1", "abc", "99999999999"} {
		if _, err := parseExpirationForm(bad); err == nil {
			t.Errorf("%q deveria dar erro", bad)
		}
	}
}
