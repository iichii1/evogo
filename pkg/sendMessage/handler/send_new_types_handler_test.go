package send_handler

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	send_service "github.com/evolution-foundation/evolution-go/pkg/sendMessage/service"
	"github.com/gin-gonic/gin"
)

type fakeSendService struct {
	send_service.SendService
	err      error
	received *send_service.ContactsStruct
}

func (f *fakeSendService) SendContacts(data *send_service.ContactsStruct, _ *instance_model.Instance) (*send_service.MessageSendStruct, error) {
	f.received = data
	return &send_service.MessageSendStruct{}, f.err
}

func callSendContacts(service *fakeSendService, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/send/contacts", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("instance", &instance_model.Instance{Id: "i1"})
	(&sendHandler{sendMessageService: service}).SendContacts(ctx)
	return recorder
}

func TestSendContactsStatusCodes(t *testing.T) {
	cases := []struct {
		name string
		body string
		err  error
		want int
	}{
		{"enviado", `{"number":"5511","contacts":[{"fullName":"A","phone":"1"}]}`, nil, http.StatusOK},
		{"JSON inválido", `{`, nil, http.StatusBadRequest},
		{"sem destino", `{"contacts":[]}`, nil, http.StatusBadRequest},
		{"cartão inválido", `{"number":"5511"}`, fmt.Errorf("%w: contacts is required", send_service.ErrInvalidSendInput), http.StatusBadRequest},
		{"erro do WhatsApp", `{"number":"5511"}`, errors.New("desconectado"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := callSendContacts(&fakeSendService{err: c.err}, c.body)
			if got.Code != c.want {
				t.Fatalf("status %d, esperava %d: %s", got.Code, c.want, got.Body)
			}
		})
	}
}

func TestSendContactsPassesTheBodyToTheService(t *testing.T) {
	service := &fakeSendService{}
	callSendContacts(service, `{"number":"5511","expiration":86400,"contacts":[{"fullName":"A","phone":"1"}]}`)
	if service.received == nil || len(service.received.Contacts) != 1 || service.received.Contacts[0].FullName != "A" {
		t.Fatalf("recebeu %+v", service.received)
	}
	if service.received.Expiration == nil || *service.received.Expiration != 86400 {
		t.Fatalf("expiration %v", service.received.Expiration)
	}
}
