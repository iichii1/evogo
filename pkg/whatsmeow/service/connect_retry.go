package whatsmeow_service

import (
	"errors"
	"io"
	"strings"
	"time"

	"go.mau.fi/util/exhttp"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/socket"

	"github.com/evolution-foundation/evolution-go/pkg/runtimestate"
)

// Uma instância já pareada que não consegue conectar por um problema passageiro (rede fora, DNS, o servidor do
// WhatsApp devolvendo 5xx) ficava parada: o StartClient só registrava o erro e saía, e nada chamava de novo. Era
// assim na subida (CONNECT_ON_STARTUP) e também depois de uma queda, porque ReconnectClient também termina em
// StartClient. Agora ele tenta de novo, com espera crescente e limite, e para na hora quando a sessão é parada
// (desconectar, sair da conta, apagar a instância).
//
// O limite existe para não insistir com o WhatsApp: são 8 tentativas a mais, a primeira após 5 s e as seguintes
// dobrando até 2 min, cerca de 8,5 minutos no total. Erros que não são passageiros (cliente desatualizado, 401,
// 403, 405, proxy recusando a senha) não são repetidos.
const (
	connectRetryBase  = 5 * time.Second
	connectRetryMax   = 2 * time.Minute
	connectRetryLimit = 8
)

// connectRetryDelay é a espera antes da tentativa extra de número retry (1 é a primeira extra).
func connectRetryDelay(retry int) time.Duration {
	if retry < 1 {
		return 0
	}
	delay := connectRetryBase << (retry - 1)
	if delay <= 0 || delay > connectRetryMax {
		return connectRetryMax
	}
	return delay
}

// isTransientConnectError diz se vale tentar de novo. Segue o critério que a própria whatsmeow usa para a
// reconexão em segundo plano (rede, 408 e 5xx do servidor, falha ao abrir o socket, tempo da negociação), mais o
// fim de arquivo, que o código antigo já repetia, e recusa o erro de senha do proxy, que tem o seu caminho.
func isTransientConnectError(err error) bool {
	if err == nil {
		return false
	}
	if strings.Contains(err.Error(), "username/password authentication failed") {
		return false
	}
	var status socket.ErrWithStatusCode
	if errors.As(err, &status) {
		switch status.StatusCode {
		case 408, 500, 501, 502, 503, 504:
			return true
		default:
			return false
		}
	}
	return exhttp.IsNetworkError(err) ||
		errors.Is(err, socket.ErrDialFailed) ||
		errors.Is(err, socket.ErrSocketClosed) ||
		errors.Is(err, whatsmeow.ErrHandshakeTimeout) ||
		errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		strings.Contains(err.Error(), "EOF")
}

// connectWithRetry conecta e, enquanto o erro for passageiro e o limite não acabar, espera e tenta de novo.
// wait devolve false quando a sessão foi parada; aí devolve o último erro sem insistir.
func connectWithRetry(connect func() error, wait func(time.Duration) bool, onRetry func(retry int, delay time.Duration, err error)) error {
	err := connect()
	for retry := 1; retry <= connectRetryLimit && isTransientConnectError(err); retry++ {
		delay := connectRetryDelay(retry)
		onRetry(retry, delay, err)
		if !wait(delay) {
			return err
		}
		err = connect()
	}
	return err
}

// connectingWithRetry diz se a instância está nessa fila de tentativas. Quem pede para iniciar a mesma instância
// nesse meio-tempo (toda chamada da API numa instância parada faz isso) não precisa esperar a trava: a fila já
// está tentando, e cada pedido a mais não deve virar uma nova fila nem uma nova conexão.
func (w whatsmeowService) connectingWithRetry(instanceID string) bool {
	return w.connectRetrying != nil && w.connectRetrying.Get(instanceID)
}

// connectPaired conecta uma instância já pareada com tentativas extras. Fica com a trava da instância durante as
// esperas, como o StartClient já fazia durante a conexão, e solta no instante em que a sessão é parada.
func (w whatsmeowService) connectPaired(instanceID string, session *runtimestate.Session, client *whatsmeow.Client) error {
	if w.connectRetrying != nil {
		w.connectRetrying.Set(instanceID, true)
		defer w.connectRetrying.Delete(instanceID)
	}
	log := w.loggerWrapper.GetLogger(instanceID)
	return connectWithRetry(client.Connect,
		func(delay time.Duration) bool {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-timer.C:
				return true
			case <-session.Stopping():
				return false
			}
		},
		func(retry int, delay time.Duration, err error) {
			log.LogWarn("[%s] Não conectou (%v); tentativa extra %d de %d em %s", instanceID, err, retry, connectRetryLimit, delay)
		})
}
