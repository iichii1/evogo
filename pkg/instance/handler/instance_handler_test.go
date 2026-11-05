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

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	instance_service "github.com/evolution-foundation/evolution-go/pkg/instance/service"
)

// fakeService implementa só o que os testes usam; o resto cai no nil da
// interface embutida e causaria panic se fosse chamado sem querer.
type fakeService struct {
	instance_service.InstanceService
	pair            func() (*instance_service.PairReturnStruct, error)
	advanced        *instance_model.AdvancedSettings
	updatedSettings *instance_model.AdvancedSettings
}

func (f *fakeService) Pair(*instance_service.PairStruct, *instance_model.Instance) (*instance_service.PairReturnStruct, error) {
	return f.pair()
}

func (f *fakeService) GetAdvancedSettings(string) (*instance_model.AdvancedSettings, error) {
	return f.advanced, nil
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
