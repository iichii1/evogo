package instance_service

import (
	"encoding/json"
	"time"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	"github.com/evolution-foundation/evolution-go/pkg/utils"
	whatsmeow_service "github.com/evolution-foundation/evolution-go/pkg/whatsmeow/service"
)

// ProxyStatus é o que se pode dizer, sem expor segredos, sobre o proxy de uma
// instância (#123).
//
// GET /instance/all e /instance/info devolvem o campo `proxy` sempre vazio, de
// propósito, para não vazar credenciais, mas então "sem proxy" e "com proxy"
// ficam iguais. E a resposta de POST /instance/proxy só prova que a configuração
// foi gravada, não que o cliente em execução a aplicou. Este objeto separa as
// duas coisas: o que está configurado e o que está de fato em uso.
//
// Nunca contém usuário, senha, URL completa nem chaves de API.
type ProxyStatus struct {
	// Configured é true se há um proxy para a instância, o dela ou o global
	// (PROXY_HOST).
	Configured bool `json:"configured" example:"true"`
	// ClientStarted é true se a instância já foi iniciada neste processo. Antes
	// disso RuntimeEnabled não diz nada.
	ClientStarted bool `json:"clientStarted" example:"true"`
	// RuntimeEnabled é true se o cliente em execução está usando o proxy.
	RuntimeEnabled bool `json:"runtimeEnabled" example:"true"`
	// Protocol, Host e Port são os efetivos (os da instância, com o global como
	// reserva).
	Protocol string `json:"protocol,omitempty" example:"http"`
	Host     string `json:"host,omitempty" example:"proxy.example.com"`
	Port     string `json:"port,omitempty" example:"8080"`
	// HasAuth é true se o proxy usa usuário e senha. Os valores não são devolvidos.
	HasAuth bool `json:"hasAuth" example:"true"`
	// FallbackWithoutProxy é true se a conexão voltou a ser direta depois de um
	// erro de autenticação do proxy.
	FallbackWithoutProxy bool `json:"fallbackWithoutProxy" example:"false"`
	// LastAppliedAt é quando o proxy foi aplicado ao cliente.
	LastAppliedAt *time.Time `json:"lastAppliedAt,omitempty"`
	// LastError é uma descrição genérica do último problema.
	LastError string `json:"lastError,omitempty"`
}

// buildProxyStatus junta a configuração gravada, a global e o estado do
// cliente em execução.
func buildProxyStatus(storedProxy string, cfg *config.Config, runtime whatsmeow_service.ProxyRuntimeStatus, started bool) ProxyStatus {
	var stored ProxyConfig
	if storedProxy != "" {
		// Configuração ilegível conta como "sem proxy da instância"; o cliente
		// também a descarta ao iniciar.
		_ = json.Unmarshal([]byte(storedProxy), &stored)
	}

	pick := func(own, global string) string {
		if own != "" {
			return own
		}
		return global
	}
	host := pick(stored.Host, cfg.ProxyHost)
	port := pick(stored.Port, cfg.ProxyPort)

	status := ProxyStatus{
		Configured:           host != "",
		ClientStarted:        started,
		RuntimeEnabled:       started && runtime.RuntimeEnabled,
		FallbackWithoutProxy: started && runtime.FallbackWithoutProxy,
		LastError:            runtime.LastError,
	}
	if started && !runtime.LastAppliedAt.IsZero() {
		appliedAt := runtime.LastAppliedAt
		status.LastAppliedAt = &appliedAt
	}
	if status.Configured {
		status.Protocol = utils.NormalizeProxyProtocol(pick(stored.Protocol, cfg.ProxyProtocol), port)
		status.Host = host
		status.Port = port
		status.HasAuth = pick(stored.Username, cfg.ProxyUsername) != ""
	}
	return status
}

// GetProxyStatus devolve o estado do proxy da instância.
func (i instances) GetProxyStatus(id string) (*ProxyStatus, error) {
	instance, err := i.instanceRepository.GetInstanceByID(id)
	if err != nil {
		return nil, err
	}

	runtime, started := i.whatsmeowService.ProxyRuntimeStatus(id)
	status := buildProxyStatus(instance.Proxy, i.config, runtime, started)
	return &status, nil
}
