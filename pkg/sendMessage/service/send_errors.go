package send_service

import (
	"errors"
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow"
)

// explainSendError acrescenta uma explicação aos erros do servidor do WhatsApp
// que costumam confundir quem usa a API. O erro original continua acessível
// com errors.Is/errors.As.
func explainSendError(err error) error {
	if err == nil || !errors.Is(err, whatsmeow.ErrServerReturnedError) {
		return err
	}
	// O whatsmeow formata como "server returned error <código>".
	if strings.HasSuffix(err.Error(), " 463") {
		// #50: o WhatsApp bloqueia mensagens desta conta para contatos que
		// ainda não falaram com ela (falta o token de privacidade do contato,
		// ou a conta está sob "reachout timelock").
		return fmt.Errorf("%w (WhatsApp refused a message to a contact that has not messaged this number first; "+
			"wait for the contact to send a message, or for the AccountReachoutTimelock event to report isActive=false)", err)
	}
	return err
}
