package instance_handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	instance_service "github.com/evolution-foundation/evolution-go/pkg/instance/service"
)

// fakeService implementa só o que os testes usam; o resto cai no nil da
// interface embutida e causaria panic se fosse chamado sem querer.
type fakeService struct {
	instance_service.InstanceService
	pair            func() (*instance_service.PairReturnStruct, error)
	advanced        *instance_model.AdvancedSettings
	advancedErr     error
	updatedSettings *instance_model.AdvancedSettings
}

func (f *fakeService) Pair(*instance_service.PairStruct, *instance_model.Instance) (*instance_service.PairReturnStruct, error) {
	return f.pair()
}

func (f *fakeService) GetAdvancedSettings(string) (*instance_model.AdvancedSettings, error) {
	return f.advanced, f.advancedErr
}

func (f *fakeService) UpdateAdvancedSettings(_ string, s *instance_model.AdvancedSettings) error {
	f.updatedSettings = s
	return nil
}

// do monta uma rota que injeta a instância autenticada (como o Auth faz) e
// executa a requisição.
func do(t *testing.T, authenticated *instance_model.Instance, method, route, url, body string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Handle(method, route, func(c *gin.Context) {
		c.Set("instance", authenticated)
		c.Next()
	}, handler)
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// Regressão de #231: uma instância já autenticada recebia 500 no /instance/pair.
func TestPairAlreadyAuthenticatedReturnsConflict(t *testing.T) {
	svc := &fakeService{pair: func() (*instance_service.PairReturnStruct, error) {
		return nil, fmt.Errorf("pair: %w", instance_service.ErrAlreadyAuthenticated)
	}}
	h := &instanceHandler{instanceService: svc}

	w := do(t, &instance_model.Instance{Id: "i1"}, http.MethodPost, "/instance/pair", "/instance/pair", `{"phone":"5511999999999"}`, h.Pair)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (%s)", w.Code, w.Body.String())
	}
	var out map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out["code"] != "already_authenticated" {
		t.Fatalf("code = %v", out["code"])
	}
	if msg, _ := out["error"].(string); msg == "" {
		t.Fatal("o campo error deve continuar presente para quem já lê a mensagem")
	}
}

func TestPairOtherErrorsStillReturn500(t *testing.T) {
	svc := &fakeService{pair: func() (*instance_service.PairReturnStruct, error) {
		return nil, errors.New("pairing failed: timeout")
	}}
	h := &instanceHandler{instanceService: svc}

	w := do(t, &instance_model.Instance{Id: "i1"}, http.MethodPost, "/instance/pair", "/instance/pair", `{"phone":"5511999999999"}`, h.Pair)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestPairSuccess(t *testing.T) {
	svc := &fakeService{pair: func() (*instance_service.PairReturnStruct, error) {
		return &instance_service.PairReturnStruct{PairingCode: "ABCD-1234"}, nil
	}}
	h := &instanceHandler{instanceService: svc}

	w := do(t, &instance_model.Instance{Id: "i1"}, http.MethodPost, "/instance/pair", "/instance/pair", `{"phone":"5511999999999"}`, h.Pair)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "ABCD-1234") {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
}

const advancedRoute = "/instance/:instanceId/advanced-settings"

// Regressão de #111: um PUT com só alguns campos zerava todos os outros.
func TestUpdateAdvancedSettingsKeepsFieldsThatWereNotSent(t *testing.T) {
	svc := &fakeService{advanced: &instance_model.AdvancedSettings{
		AlwaysOnline:  false,
		RejectCall:    true,
		MsgRejectCall: "ocupado",
		ReadMessages:  true,
		IgnoreGroups:  true,
		IgnoreStatus:  true,
	}}
	h := &instanceHandler{instanceService: svc}

	w := do(t, &instance_model.Instance{Id: "i1"}, http.MethodPut, advancedRoute, "/instance/i1/advanced-settings", `{"alwaysOnline":true}`, h.UpdateAdvancedSettings)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", w.Code, w.Body.String())
	}
	got := svc.updatedSettings
	if got == nil {
		t.Fatal("UpdateAdvancedSettings não foi chamado")
	}
	want := instance_model.AdvancedSettings{
		AlwaysOnline: true, RejectCall: true, MsgRejectCall: "ocupado",
		ReadMessages: true, IgnoreGroups: true, IgnoreStatus: true,
	}
	if *got != want {
		t.Fatalf("settings salvas = %+v, want %+v", *got, want)
	}
}

func TestUpdateAdvancedSettingsCanTurnFlagsOffAndClearMessage(t *testing.T) {
	svc := &fakeService{advanced: &instance_model.AdvancedSettings{RejectCall: true, MsgRejectCall: "ocupado", IgnoreGroups: true}}
	h := &instanceHandler{instanceService: svc}

	w := do(t, &instance_model.Instance{Id: "i1"}, http.MethodPut, advancedRoute, "/instance/i1/advanced-settings",
		`{"rejectCall":false,"msgRejectCall":""}`, h.UpdateAdvancedSettings)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	got := svc.updatedSettings
	if got.RejectCall || got.MsgRejectCall != "" {
		t.Fatalf("false/\"\" enviados de propósito precisam valer: %+v", *got)
	}
	if !got.IgnoreGroups {
		t.Fatal("ignoreGroups não foi enviado e devia ser mantido")
	}
}

func TestUpdateAdvancedSettingsFullBodyStillWorks(t *testing.T) {
	svc := &fakeService{advanced: &instance_model.AdvancedSettings{}}
	h := &instanceHandler{instanceService: svc}

	body := `{"alwaysOnline":true,"rejectCall":true,"msgRejectCall":"x","readMessages":true,"ignoreGroups":true,"ignoreStatus":true}`
	w := do(t, &instance_model.Instance{Id: "i1"}, http.MethodPut, advancedRoute, "/instance/i1/advanced-settings", body, h.UpdateAdvancedSettings)

	want := instance_model.AdvancedSettings{AlwaysOnline: true, RejectCall: true, MsgRejectCall: "x", ReadMessages: true, IgnoreGroups: true, IgnoreStatus: true}
	if w.Code != http.StatusOK || *svc.updatedSettings != want {
		t.Fatalf("status = %d, settings = %+v", w.Code, svc.updatedSettings)
	}
}

// O apikey de uma instância não pode ler nem alterar as configurações de outra.
func TestAdvancedSettingsRejectsAnotherInstancesID(t *testing.T) {
	svc := &fakeService{advanced: &instance_model.AdvancedSettings{}}
	h := &instanceHandler{instanceService: svc}
	other := &instance_model.Instance{Id: "instancia-A"}

	w := do(t, other, http.MethodGet, advancedRoute, "/instance/instancia-B/advanced-settings", "", h.GetAdvancedSettings)
	if w.Code != http.StatusForbidden {
		t.Fatalf("GET de outra instância: status = %d, want 403", w.Code)
	}

	w = do(t, other, http.MethodPut, advancedRoute, "/instance/instancia-B/advanced-settings", `{"alwaysOnline":true}`, h.UpdateAdvancedSettings)
	if w.Code != http.StatusForbidden {
		t.Fatalf("PUT de outra instância: status = %d, want 403", w.Code)
	}
	if svc.updatedSettings != nil {
		t.Fatal("o PUT negado não pode ter gravado nada")
	}
}

func TestAdvancedSettingsGetOwnInstance(t *testing.T) {
	svc := &fakeService{advanced: &instance_model.AdvancedSettings{AlwaysOnline: true}}
	h := &instanceHandler{instanceService: svc}

	w := do(t, &instance_model.Instance{Id: "i1"}, http.MethodGet, advancedRoute, "/instance/i1/advanced-settings", "", h.GetAdvancedSettings)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"alwaysOnline":true`) {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
}

// A chave global age sobre qualquer instância (#81).
func TestAdvancedSettingsAdminCanManageAnyInstance(t *testing.T) {
	svc := &fakeService{advanced: &instance_model.AdvancedSettings{}}
	h := &instanceHandler{instanceService: svc}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.PUT(advancedRoute, func(c *gin.Context) { c.Set("admin", true); c.Next() }, h.UpdateAdvancedSettings)
	req := httptest.NewRequest(http.MethodPut, "/instance/qualquer/advanced-settings", strings.NewReader(`{"alwaysOnline":true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK || svc.updatedSettings == nil || !svc.updatedSettings.AlwaysOnline {
		t.Fatalf("status = %d settings = %+v", w.Code, svc.updatedSettings)
	}
}

func TestAdvancedSettingsUnknownInstanceIs404(t *testing.T) {
	svc := &fakeService{advancedErr: gorm.ErrRecordNotFound}
	h := &instanceHandler{instanceService: svc}

	w := do(t, &instance_model.Instance{Id: "i1"}, http.MethodGet, advancedRoute, "/instance/i1/advanced-settings", "", h.GetAdvancedSettings)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}
