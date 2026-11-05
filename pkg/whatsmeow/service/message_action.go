package whatsmeow_service

import (
	"context"
	"fmt"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
)

// secretDecrypter é o pedaço do cliente do WhatsApp que decifra mensagens
// protegidas por segredo (edição de mensagem, de evento e de enquete).
type secretDecrypter interface {
	DecryptSecretEncryptedMessage(ctx context.Context, evt *events.Message) (*waE2E.Message, error)
}

// messageAction descreve o que uma mensagem faz com outra: editar ou apagar.
//
// O webhook de uma edição trazia só o `secretEncryptedMessage` cifrado, com
// `IsEdit: false`, e o de uma exclusão só um `protocolMessage` de tipo 0; quem
// consumia o evento não tinha como ler o texto novo nem saber, sem decorar os
// números, que era uma edição ou uma exclusão (#92, #62, #146). O resultado vai
// em `data.messageAction`, campo novo, sem mexer nos que já existiam.
//
//	{"type": "edit", "targetMessageId": "3EB0...", "decrypted": true, "message": {...conteúdo novo...}}
//	{"type": "revoke", "targetMessageId": "3EB0..."}
type messageAction map[string]interface{}

// buildMessageAction devolve a ação da mensagem, ou nil se ela não edita nem
// apaga nada. Falha ao decifrar não perde o evento: a ação sai com
// `decrypted: false`.
func buildMessageAction(ctx context.Context, decrypter secretDecrypter, evt *events.Message) messageAction {
	if evt == nil || evt.Message == nil {
		return nil
	}

	if enc := evt.Message.GetSecretEncryptedMessage(); enc != nil {
		action := messageAction{
			"type":            secretEncTypeName(enc.GetSecretEncType()),
			"targetMessageId": enc.GetTargetMessageKey().GetID(),
			"decrypted":       false,
		}
		if decrypter == nil {
			return action
		}
		decrypted, err := decrypter.DecryptSecretEncryptedMessage(ctx, evt)
		if err != nil {
			action["error"] = fmt.Sprintf("%v", err)
			return action
		}
		action["decrypted"] = true
		action["message"] = decrypted
		return action
	}

	if protocol := evt.Message.GetProtocolMessage(); protocol != nil &&
		protocol.GetType() == waE2E.ProtocolMessage_REVOKE && protocol.GetKey().GetID() != "" {
		return messageAction{
			"type":            "revoke",
			"targetMessageId": protocol.GetKey().GetID(),
		}
	}

	return nil
}

func secretEncTypeName(t waE2E.SecretEncryptedMessage_SecretEncType) string {
	switch t {
	case waE2E.SecretEncryptedMessage_MESSAGE_EDIT:
		return "edit"
	case waE2E.SecretEncryptedMessage_EVENT_EDIT:
		return "eventEdit"
	case waE2E.SecretEncryptedMessage_POLL_EDIT:
		return "pollEdit"
	case waE2E.SecretEncryptedMessage_POLL_ADD_OPTION:
		return "pollAddOption"
	default:
		return "secretEncrypted"
	}
}

// isEdit informa se a ação é a edição de uma mensagem.
func (a messageAction) isEdit() bool {
	return a != nil && a["type"] == "edit"
}
