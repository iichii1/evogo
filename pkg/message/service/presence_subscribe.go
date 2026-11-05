package message_service

import (
	"context"
	"errors"
	"fmt"

	"github.com/evolution-foundation/evolution-go/pkg/utils"
	"go.mau.fi/whatsmeow/types"
)

// maxPresenceSubscriptions limita quantos contatos uma chamada assina. O pedido
// típico é "os N contatos mais recentes da caixa de entrada"; centenas de
// assinaturas de uma vez só serviriam para sobrecarregar a conta.
const maxPresenceSubscriptions = 50

// ErrSubscribeRequest marca os erros de pedido inválido de SubscribePresence
// (o handler responde 400 em vez de 500).
var ErrSubscribeRequest = errors.New("invalid subscribe presence request")

// IsSubscribeRequestError informa se o erro veio de um pedido inválido.
func IsSubscribeRequestError(err error) bool {
	return errors.Is(err, ErrSubscribeRequest)
}

// SubscribePresenceStruct é o corpo de POST /message/presence/subscribe. Informe
// `number` ou `numbers`.
type SubscribePresenceStruct struct {
	// Número do contato (ou JID).
	Number string `json:"number,omitempty" example:"5511999999999"`
	// Vários contatos de uma vez (no máximo 50).
	Numbers []string `json:"numbers,omitempty"`
}

// SubscribePresenceResult diz quais contatos foram assinados e quais falharam.
type SubscribePresenceResult struct {
	Subscribed []string                   `json:"subscribed"`
	Failed     []SubscribePresenceFailure `json:"failed,omitempty"`
}

// SubscribePresenceFailure é um contato que não pôde ser assinado.
type SubscribePresenceFailure struct {
	Number string `json:"number"`
	Error  string `json:"error"`
}

type presenceSubscriber interface {
	presenceSender
	SubscribePresence(ctx context.Context, jid types.JID) error
}

// presenceTargets lê, valida e remove repetidos dos contatos do pedido.
func presenceTargets(data *SubscribePresenceStruct) (valid []types.JID, invalid []SubscribePresenceFailure, err error) {
	var raw []string
	if data.Number != "" {
		raw = append(raw, data.Number)
	}
	raw = append(raw, data.Numbers...)

	if len(raw) == 0 {
		return nil, nil, fmt.Errorf("%w: number or numbers is required", ErrSubscribeRequest)
	}
	if len(raw) > maxPresenceSubscriptions {
		return nil, nil, fmt.Errorf("%w: at most %d numbers per request", ErrSubscribeRequest, maxPresenceSubscriptions)
	}

	seen := map[types.JID]bool{}
	for _, number := range raw {
		jid, ok := utils.ParseJID(number)
		if !ok {
			invalid = append(invalid, SubscribePresenceFailure{Number: number, Error: "invalid phone number"})
			continue
		}
		// A assinatura é um nó cru, sem normalização pelo usync: precisa do JID
		// canônico, só dígitos, como o chatstate (veja utils.CanonicalJID).
		jid = utils.CanonicalJID(jid)
		if seen[jid] {
			continue
		}
		seen[jid] = true
		valid = append(valid, jid)
	}

	if len(valid) == 0 {
		return nil, nil, fmt.Errorf("%w: no valid phone number", ErrSubscribeRequest)
	}
	return valid, invalid, nil
}

// subscribePresence fica online (o WhatsApp só entrega a presença dos outros a
// quem está online) e assina cada contato. A falha de um não impede os demais.
func subscribePresence(ctx context.Context, client presenceSubscriber, jids []types.JID, invalid []SubscribePresenceFailure) (*SubscribePresenceResult, error) {
	result := &SubscribePresenceResult{Subscribed: []string{}, Failed: invalid}

	if err := client.SendPresence(ctx, types.PresenceAvailable); err != nil {
		return nil, fmt.Errorf("failed to go online before subscribing: %w", err)
	}

	for _, jid := range jids {
		if err := client.SubscribePresence(ctx, jid); err != nil {
			result.Failed = append(result.Failed, SubscribePresenceFailure{Number: jid.String(), Error: err.Error()})
			continue
		}
		result.Subscribed = append(result.Subscribed, jid.String())
	}
	return result, nil
}
