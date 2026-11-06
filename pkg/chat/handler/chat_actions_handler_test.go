package chat_handler

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	chat_service "github.com/evolution-foundation/evolution-go/pkg/chat/service"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"github.com/gin-gonic/gin"
)

type fakeChatService struct {
	chat_service.ChatService
	err      error
	received *chat_service.ChatRangeStruct
}

func (f *fakeChatService) ChatMarkUnread(data *chat_service.ChatRangeStruct, _ *instance_model.Instance) error {
	f.received = data
	return f.err
}

func callMarkUnread(t *testing.T, service *fakeChatService, body string, withInstance bool) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/chat/markunread", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	if withInstance {
		ctx.Set("instance", &instance_model.Instance{Id: "i1"})
	} else {
		ctx.Set("instance", "não é uma instância")
	}
	(&chatHandler{chatService: service}).ChatMarkUnread(ctx)
	return recorder
}

func TestChatActionStatusCodes(t *testing.T) {
	cases := []struct {
		name string
		body string
		err  error
		want int
	}{
		{"sucesso", `{"chat":"5511@s.whatsapp.net"}`, nil, http.StatusOK},
		{"JSON inválido", `{`, nil, http.StatusBadRequest},
		{"o serviço recusa", `{"chat":""}`, fmt.Errorf("%w: invalid chat", chat_service.ErrInvalidChatAction), http.StatusBadRequest},
		{"erro do WhatsApp", `{"chat":"5511@s.whatsapp.net"}`, errors.New("desconectado"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := callMarkUnread(t, &fakeChatService{err: c.err}, c.body, true)
			if got.Code != c.want {
				t.Fatalf("status %d, esperava %d: %s", got.Code, c.want, got.Body)
			}
		})
	}
}

func TestChatActionPassesTheBodyToTheService(t *testing.T) {
	service := &fakeChatService{}
	callMarkUnread(t, service, `{"chat":"5511@s.whatsapp.net","lastMessageId":"M1","lastMessageTimestamp":1700000000}`, true)
	if service.received == nil || service.received.Chat != "5511@s.whatsapp.net" || service.received.LastMessageId != "M1" || service.received.LastMessageTimestamp != 1700000000 {
		t.Fatalf("recebeu %+v", service.received)
	}
}

func TestChatActionWithoutInstanceIsRefused(t *testing.T) {
	service := &fakeChatService{}
	got := callMarkUnread(t, service, `{"chat":"x"}`, false)
	if got.Code != http.StatusInternalServerError || service.received != nil {
		t.Fatalf("status %d, recebeu %+v", got.Code, service.received)
	}
}
