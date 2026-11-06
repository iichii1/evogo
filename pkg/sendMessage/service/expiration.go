package send_service

import "go.mau.fi/whatsmeow/proto/waE2E"

// applyExpiration grava o tempo das mensagens temporárias no contexto da mensagem
// (#79). A whatsmeow não guarda o tempo de cada conversa, então quem envia informa
// o valor visto nas mensagens recebidas (`contextInfo.expiration`) ou em /group/info.
// Sem ele, a mensagem sai sem prazo mesmo numa conversa temporária.
func applyExpiration(msg *waE2E.Message, expiration *uint32) {
	if expiration == nil || *expiration == 0 {
		return
	}
	if info := messageContextInfo(msg); info != nil {
		info.Expiration = expiration
	}
}

// messageContextInfo devolve o contexto já criado por SendMessage, ou nil para os
// formatos que não passam por ele.
func messageContextInfo(msg *waE2E.Message) *waE2E.ContextInfo {
	switch {
	case msg.ExtendedTextMessage != nil:
		return msg.ExtendedTextMessage.ContextInfo
	case msg.ImageMessage != nil:
		return msg.ImageMessage.ContextInfo
	case msg.VideoMessage != nil:
		return msg.VideoMessage.ContextInfo
	case msg.PtvMessage != nil:
		return msg.PtvMessage.ContextInfo
	case msg.AudioMessage != nil:
		return msg.AudioMessage.ContextInfo
	case msg.DocumentMessage != nil:
		return msg.DocumentMessage.ContextInfo
	case msg.DocumentWithCaptionMessage != nil:
		return msg.DocumentWithCaptionMessage.GetMessage().GetDocumentMessage().GetContextInfo()
	case msg.PollCreationMessage != nil:
		return msg.PollCreationMessage.ContextInfo
	case msg.StickerMessage != nil:
		return msg.StickerMessage.ContextInfo
	case msg.LocationMessage != nil:
		return msg.LocationMessage.ContextInfo
	case msg.ContactMessage != nil:
		return msg.ContactMessage.ContextInfo
	case msg.ContactsArrayMessage != nil:
		return msg.ContactsArrayMessage.ContextInfo
	case msg.GroupInviteMessage != nil:
		return msg.GroupInviteMessage.ContextInfo
	case msg.RequestPhoneNumberMessage != nil:
		return msg.RequestPhoneNumberMessage.ContextInfo
	}
	return nil
}
