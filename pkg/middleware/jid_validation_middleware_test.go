package auth_middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// serve monta uma rota com o middleware e devolve o body que o handler recebeu.
func serve(t *testing.T, body string, fields ...string) (status int, received map[string]interface{}, errMsg string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/x", NewJIDValidationMiddleware().ValidateJIDFields(fields...), func(c *gin.Context) {
		var m map[string]interface{}
		if err := c.ShouldBindJSON(&m); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, m)
	})

	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var out map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != http.StatusOK {
		msg, _ := out["error"].(string)
		return w.Code, nil, msg
	}
	return w.Code, out, ""
}

// Regressão de #52/#97: o exemplo documentado de POST /group/participant
// (participants é uma lista) sempre voltava 400.
func TestValidateJIDFieldsAcceptsArrayOfNumbers(t *testing.T) {
	status, got, msg := serve(t,
		`{"groupJid":"120363000000000000@g.us","action":"add","participants":["5511999999999","5511888888888"]}`,
		"participants")
	if status != http.StatusOK {
		t.Fatalf("status = %d (%s), want 200", status, msg)
	}
	list, ok := got["participants"].([]interface{})
	if !ok || len(list) != 2 {
		t.Fatalf("participants = %#v", got["participants"])
	}
	for _, item := range list {
		if !strings.HasSuffix(item.(string), "@s.whatsapp.net") {
			t.Fatalf("participante não foi normalizado para JID: %v", item)
		}
	}
	if got["groupJid"] != "120363000000000000@g.us" {
		t.Fatalf("outros campos devem passar intactos: %v", got["groupJid"])
	}
}

func TestValidateJIDFieldsKeepsAlreadyValidArrayUntouched(t *testing.T) {
	status, got, msg := serve(t, `{"participants":["5511999999999@s.whatsapp.net"]}`, "participants")
	if status != http.StatusOK {
		t.Fatalf("status = %d (%s)", status, msg)
	}
	if list := got["participants"].([]interface{}); list[0] != "5511999999999@s.whatsapp.net" {
		t.Fatalf("participants = %v", list)
	}
}

func TestValidateJIDFieldsStillValidatesSingleStrings(t *testing.T) {
	status, got, _ := serve(t, `{"number":"5511999999999"}`, "number")
	if status != http.StatusOK || !strings.HasSuffix(got["number"].(string), "@s.whatsapp.net") {
		t.Fatalf("status = %d, number = %v", status, got["number"])
	}
	if status, _, msg := serve(t, `{"number":""}`, "number"); status != http.StatusBadRequest || !strings.Contains(msg, "number is required") {
		t.Fatalf("string vazia: status = %d, msg = %q", status, msg)
	}
}

func TestValidateJIDFieldsRejectsBadInput(t *testing.T) {
	cases := []struct {
		name, body, wantMsg string
	}{
		{"lista vazia", `{"participants":[]}`, "participants cannot be empty"},
		{"item vazio", `{"participants":["5511999999999",""]}`, "participants[1] must be a non-empty string"},
		{"item que não é string", `{"participants":[5511999999999]}`, "participants[0] must be a non-empty string"},
		{"item inválido", `{"participants":["abc"]}`, "Invalid participants[0] format"},
		{"null", `{"participants":null}`, "participants is required"},
		{"número JSON", `{"participants":5511999999999}`, "participants must be a string or an array of strings"},
		{"objeto", `{"participants":{"a":1}}`, "participants must be a string or an array of strings"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, _, msg := serve(t, tc.body, "participants")
			if status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", status)
			}
			if !strings.Contains(msg, tc.wantMsg) {
				t.Fatalf("mensagem = %q, quero conter %q", msg, tc.wantMsg)
			}
		})
	}
}

func TestValidateJIDFieldsIgnoresAbsentFields(t *testing.T) {
	if status, _, msg := serve(t, `{"groupJid":"120363000000000000@g.us"}`, "number", "participants"); status != http.StatusOK {
		t.Fatalf("campos ausentes não devem falhar: %d %s", status, msg)
	}
}
