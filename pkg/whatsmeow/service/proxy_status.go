package whatsmeow_service

import "time"

// ProxyRuntimeStatus descreve o que aconteceu com o proxy no cliente em
// execução da instância (#123).
//
// A resposta de POST /instance/proxy só prova que a configuração foi gravada.
// O cliente pode não ter aplicado o proxy (configuração inválida) ou ter caído
// para a conexão direta depois de um erro de autenticação do proxy, e sem esta
// informação um sistema externo que confia só no HTTP 200 ou no estado
// "conectado" daria um falso positivo.
type ProxyRuntimeStatus struct {
	// RuntimeEnabled é true se o cliente em execução está usando o proxy.
	RuntimeEnabled bool
	// FallbackWithoutProxy é true se a conexão foi refeita sem proxy depois de
	// um erro de autenticação do proxy.
	FallbackWithoutProxy bool
	// LastAppliedAt é quando o proxy foi aplicado ao cliente (zero se nunca).
	LastAppliedAt time.Time
	// LastError é uma descrição genérica do último problema, sem endereço nem
	// credenciais.
	LastError string
}

// ProxyRuntimeStatus devolve o estado do proxy no cliente da instância e se
// há algum cliente (ou seja, se a instância já foi iniciada neste processo).
func (w *whatsmeowService) ProxyRuntimeStatus(instanceId string) (ProxyRuntimeStatus, bool) {
	if w.proxyStatus == nil {
		return ProxyRuntimeStatus{}, false
	}
	status, ok := w.proxyStatus.Load(instanceId)
	if !ok || status == nil {
		return ProxyRuntimeStatus{}, false
	}
	return *status, true
}

func (w whatsmeowService) setProxyStatus(instanceId string, status ProxyRuntimeStatus) {
	if w.proxyStatus == nil {
		return
	}
	w.proxyStatus.Set(instanceId, &status)
}

// markProxyFallback registra que o cliente passou a conectar sem o proxy.
func (w whatsmeowService) markProxyFallback(instanceId string) {
	current, _ := w.ProxyRuntimeStatus(instanceId)
	current.RuntimeEnabled = false
	current.FallbackWithoutProxy = true
	current.LastError = "proxy authentication failed; connected without proxy"
	w.setProxyStatus(instanceId, current)
}
