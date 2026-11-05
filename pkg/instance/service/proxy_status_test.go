package instance_service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	whatsmeow_service "github.com/evolution-foundation/evolution-go/pkg/whatsmeow/service"
)

const storedProxy = `{"protocol":"http","host":"proxy.example.com","port":"8080","username":"cliente-123","password":"s3nh4-s3cr3t4"}`

func TestBuildProxyStatusReportsWhatIsRunning(t *testing.T) {
	appliedAt := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)

	got := buildProxyStatus(storedProxy, &config.Config{}, whatsmeow_service.ProxyRuntimeStatus{
		RuntimeEnabled: true,
		LastAppliedAt:  appliedAt,
	}, true)

	if !got.Configured || !got.ClientStarted || !got.RuntimeEnabled || got.FallbackWithoutProxy {
		t.Fatalf("status = %+v", got)
	}
	if got.Protocol != "http" || got.Host != "proxy.example.com" || got.Port != "8080" || !got.HasAuth {
		t.Fatalf("dados do proxy = %+v", got)
	}
	if got.LastAppliedAt == nil || !got.LastAppliedAt.Equal(appliedAt) {
		t.Fatalf("lastAppliedAt = %v", got.LastAppliedAt)
	}
}

// O objetivo do #123: HTTP 200 e "conectado" não provam que o proxy está em uso.
func TestBuildProxyStatusShowsAFallbackToTheDirectConnection(t *testing.T) {
	got := buildProxyStatus(storedProxy, &config.Config{}, whatsmeow_service.ProxyRuntimeStatus{
		FallbackWithoutProxy: true,
		LastError:            "proxy authentication failed; connected without proxy",
	}, true)

	if !got.Configured {
		t.Error("o proxy continua configurado")
	}
	if got.RuntimeEnabled {
		t.Error("runtimeEnabled deveria ser false depois do fallback")
	}
	if !got.FallbackWithoutProxy || got.LastError == "" {
		t.Errorf("o fallback e o motivo deveriam aparecer: %+v", got)
	}
}

func TestBuildProxyStatusNeverContainsCredentials(t *testing.T) {
	got := buildProxyStatus(storedProxy, &config.Config{}, whatsmeow_service.ProxyRuntimeStatus{RuntimeEnabled: true}, true)

	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"cliente-123", "s3nh4-s3cr3t4", "password", "username", "http://"} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("a resposta contém %q: %s", secret, raw)
		}
	}
}

func TestBuildProxyStatusWithoutProxy(t *testing.T) {
	got := buildProxyStatus("", &config.Config{}, whatsmeow_service.ProxyRuntimeStatus{}, true)

	if got.Configured || got.RuntimeEnabled || got.Host != "" || got.HasAuth {
		t.Fatalf("instância sem proxy: %+v", got)
	}
	// "Sem proxy" e "proxy que a API escondeu" agora são distinguíveis.
	if raw, _ := json.Marshal(got); !strings.Contains(string(raw), `"configured":false`) {
		t.Errorf("configured:false deveria aparecer: %s", raw)
	}
}

func TestBuildProxyStatusUsesTheGlobalProxyAsFallback(t *testing.T) {
	cfg := &config.Config{ProxyHost: "global.example.com", ProxyPort: "3128", ProxyProtocol: "http", ProxyUsername: "global-user"}

	got := buildProxyStatus("", cfg, whatsmeow_service.ProxyRuntimeStatus{RuntimeEnabled: true}, true)

	if !got.Configured || got.Host != "global.example.com" || got.Port != "3128" || !got.HasAuth {
		t.Fatalf("status = %+v", got)
	}

	// Host da instância vence o global; o resto herda.
	got = buildProxyStatus(`{"host":"da-instancia.example.com"}`, cfg, whatsmeow_service.ProxyRuntimeStatus{}, true)
	if got.Host != "da-instancia.example.com" || got.Port != "3128" {
		t.Fatalf("status = %+v", got)
	}
}

// Instância ainda não iniciada neste processo: não se afirma nada sobre o runtime.
func TestBuildProxyStatusBeforeTheClientStarts(t *testing.T) {
	got := buildProxyStatus(storedProxy, &config.Config{}, whatsmeow_service.ProxyRuntimeStatus{RuntimeEnabled: true}, false)

	if got.ClientStarted || got.RuntimeEnabled {
		t.Fatalf("sem cliente, runtimeEnabled não pode ser true: %+v", got)
	}
	if !got.Configured {
		t.Error("o proxy gravado continua configurado")
	}
}

func TestBuildProxyStatusIgnoresAnUnreadableStoredProxy(t *testing.T) {
	got := buildProxyStatus("{não é json", &config.Config{}, whatsmeow_service.ProxyRuntimeStatus{}, true)
	if got.Configured {
		t.Fatalf("status = %+v", got)
	}
}
