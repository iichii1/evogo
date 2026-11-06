package group_handler

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	group_service "github.com/evolution-foundation/evolution-go/pkg/group/service"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"github.com/gin-gonic/gin"
	"go.mau.fi/whatsmeow/types"
)

type fakeGroupService struct {
	group_service.GroupService
	err      error
	updated  *group_service.UpdateGroupRequestParticipantsStruct
	listed   bool
	joined   bool
	response []types.GroupParticipant
}

func (f *fakeGroupService) GetGroupRequestParticipants(*group_service.GetGroupRequestParticipantsStruct, *instance_model.Instance) ([]group_service.EnrichedGroupParticipantRequest, error) {
	f.listed = true
	return nil, f.err
}

func (f *fakeGroupService) UpdateGroupRequestParticipants(data *group_service.UpdateGroupRequestParticipantsStruct, _ *instance_model.Instance) ([]types.GroupParticipant, error) {
	f.updated = data
	return f.response, f.err
}

func (f *fakeGroupService) JoinGroupWithInvite(*group_service.GroupInviteStruct, *instance_model.Instance) error {
	f.joined = true
	return f.err
}

func callGroup(handler gin.HandlerFunc, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("instance", &instance_model.Instance{Id: "i1"})
	handler(ctx)
	return recorder
}

func TestUpdateGroupRequestsValidatesBeforeCallingTheService(t *testing.T) {
	bad := map[string]string{
		"sem grupo":     `{"action":"approve","participants":["5511"]}`,
		"ação inválida": `{"groupJid":"1@g.us","action":"promote","participants":["5511"]}`,
		"sem pessoas":   `{"groupJid":"1@g.us","action":"approve"}`,
		"JSON inválido": `{`,
	}
	for name, body := range bad {
		service := &fakeGroupService{}
		got := callGroup((&groupHandler{groupService: service}).UpdateGroupRequests, body)
		if got.Code != http.StatusBadRequest || service.updated != nil {
			t.Errorf("%s: status %d, chamou o serviço: %v", name, got.Code, service.updated != nil)
		}
	}
}

func TestUpdateGroupRequestsCallsTheService(t *testing.T) {
	service := &fakeGroupService{}
	got := callGroup((&groupHandler{groupService: service}).UpdateGroupRequests, `{"groupJid":"1@g.us","action":"reject","participants":["5511"]}`)
	if got.Code != http.StatusOK {
		t.Fatalf("status %d: %s", got.Code, got.Body)
	}
	if service.updated == nil || service.updated.Action != "reject" || service.updated.Participants[0] != "5511" {
		t.Fatalf("serviço recebeu %+v", service.updated)
	}
}

func TestGetGroupRequestsRequiresTheGroup(t *testing.T) {
	service := &fakeGroupService{}
	handler := (&groupHandler{groupService: service}).GetGroupRequests
	if got := callGroup(handler, `{}`); got.Code != http.StatusBadRequest || service.listed {
		t.Fatalf("status %d, chamou o serviço: %v", got.Code, service.listed)
	}
	if got := callGroup(handler, `{"groupJid":"1@g.us"}`); got.Code != http.StatusOK || !service.listed {
		t.Fatalf("status %d, chamou o serviço: %v", got.Code, service.listed)
	}
}

func TestJoinGroupWithInviteStatusCodes(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"entrou", nil, http.StatusOK},
		{"convite inválido", fmt.Errorf("%w: code is required", group_service.ErrInvalidGroupInvite), http.StatusBadRequest},
		{"erro do WhatsApp", errors.New("convite vencido"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			service := &fakeGroupService{err: c.err}
			got := callGroup((&groupHandler{groupService: service}).JoinGroupWithInvite, `{"groupJid":"1@g.us"}`)
			if got.Code != c.want || !service.joined {
				t.Fatalf("status %d, esperava %d", got.Code, c.want)
			}
		})
	}
}
