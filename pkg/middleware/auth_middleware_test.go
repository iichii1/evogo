package auth_middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	instance_service "github.com/evolution-foundation/evolution-go/pkg/instance/service"
)

type tokenLookup struct {
	instance_service.InstanceService
	byToken map[string]*instance_model.Instance
}

func (t tokenLookup) GetInstanceByToken(token string) (*instance_model.Instance, error) {
	if inst, ok := t.byToken[token]; ok {
		return inst, nil
	}
	return nil, errors.New("not found")
}

// Regressão de #81: a documentação manda usar a chave global em
// /instance/:id/advanced-settings, mas a rota só aceitava o token da instância.
func TestAuthInstanceOrAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := NewMiddleware(&config.Config{GlobalApiKey: "chave-global"}, tokenLookup{
		byToken: map[string]*instance_model.Instance{"token-a": {Id: "A"}},
	})

	type result struct {
		status   int
		admin    bool
		instance string
	}
	call := func(apikey string) result {
		var got result
		r := gin.New()
		r.GET("/x", m.AuthInstanceOrAdmin, func(c *gin.Context) {
			got.admin = IsAdmin(c)
			if v, ok := c.Get("instance"); ok {
				got.instance = v.(*instance_model.Instance).Id
			}
			c.Status(http.StatusOK)
		})
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		if apikey != "" {
			req.Header.Set("apikey", apikey)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		got.status = w.Code
		return got
	}

	if got := call("chave-global"); got.status != 200 || !got.admin || got.instance != "" {
		t.Fatalf("chave global: %+v", got)
	}
	if got := call("token-a"); got.status != 200 || got.admin || got.instance != "A" {
		t.Fatalf("token da instância: %+v", got)
	}
	if got := call("qualquer-outra"); got.status != http.StatusUnauthorized {
		t.Fatalf("token desconhecido: %+v", got)
	}
	if got := call(""); got.status != http.StatusUnauthorized {
		t.Fatalf("sem apikey: %+v", got)
	}
}

// Sem chave global configurada, um apikey vazio ou qualquer coisa não pode virar admin.
func TestAuthInstanceOrAdminWithoutGlobalKeyConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := NewMiddleware(&config.Config{}, tokenLookup{})
	r := gin.New()
	r.GET("/x", m.AuthInstanceOrAdmin, func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("apikey", "x")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}
