package instance_model

import (
	"fmt"
	"strings"
	"testing"
)

// A conexão registrava a instância inteira no log, com o token (ensaio de 2026-11-06).
func TestPrintingAnInstanceHidesItsSecrets(t *testing.T) {
	instance := &Instance{
		Id:      "instance-id",
		Name:    "Vendas",
		Token:   "token-secreto",
		Webhook: "http://usuario:senha-webhook@destino/webhook",
		Proxy:   `{"password":"senha-proxy"}`,
		Qrcode:  "codigo-do-qr",
		Jid:     "5511000000001@s.whatsapp.net",
	}

	for _, format := range []string{"%v", "%+v", "%s"} {
		for _, value := range []interface{}{instance, *instance} {
			printed := fmt.Sprintf(format, value)
			for _, secret := range []string{"token-secreto", "senha-webhook", "senha-proxy", "codigo-do-qr", "5511000000001"} {
				if strings.Contains(printed, secret) {
					t.Errorf("%s mostrou %q: %s", format, secret, printed)
				}
			}
			if !strings.Contains(printed, "instance-id") {
				t.Errorf("%s deveria identificar a instância: %s", format, printed)
			}
		}
	}
}
