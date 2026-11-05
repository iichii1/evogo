package send_service

import "go.mau.fi/whatsmeow/proto/waE2E"

// mentionContextInfo devolve o ContextInfo (criando se não existir) da mensagem
// do tipo informado, ou nil se o tipo não aceita menções.
//
// Um documento com legenda não vem em msg.DocumentMessage: vem embrulhado em
// msg.DocumentWithCaptionMessage. Ler só o primeiro dava um ponteiro nulo e
// derrubava o envio de documento com legenda e menção (#114).
func mentionContextInfo(msg *waE2E.Message, messageType string) *waE2E.ContextInfo {
	ensure := func(ci **waE2E.ContextInfo) *waE2E.ContextInfo {
		if *ci == nil {
			*ci = &waE2E.ContextInfo{}
		}
		return *ci
	}

	switch messageType {
	case "ExtendedTextMessage":
		if m := msg.ExtendedTextMessage; m != nil {
			return ensure(&m.ContextInfo)
		}
	case "ImageMessage":
		if m := msg.ImageMessage; m != nil {
			return ensure(&m.ContextInfo)
		}
	case "VideoMessage":
		if m := msg.VideoMessage; m != nil {
			return ensure(&m.ContextInfo)
		}
	case "PtvMessage":
		if m := msg.PtvMessage; m != nil {
			return ensure(&m.ContextInfo)
		}
	case "AudioMessage":
		if m := msg.AudioMessage; m != nil {
			return ensure(&m.ContextInfo)
		}
	case "DocumentMessage":
		if m := msg.DocumentMessage; m != nil {
			return ensure(&m.ContextInfo)
		}
		if m := msg.GetDocumentWithCaptionMessage().GetMessage().GetDocumentMessage(); m != nil {
			return ensure(&m.ContextInfo)
		}
	case "PollCreationMessage":
		if m := msg.PollCreationMessage; m != nil {
			return ensure(&m.ContextInfo)
		}
	case "StickerMessage":
		if m := msg.StickerMessage; m != nil {
			return ensure(&m.ContextInfo)
		}
	case "LocationMessage":
		if m := msg.LocationMessage; m != nil {
			return ensure(&m.ContextInfo)
		}
	case "ContactMessage":
		if m := msg.ContactMessage; m != nil {
			return ensure(&m.ContextInfo)
		}
	}
	return nil
}

// setMentions grava os JIDs mencionados na mensagem. Tipos que não aceitam
// menção (botões, listas, carrossel) ficam como estão.
func setMentions(msg *waE2E.Message, messageType string, jids []string) {
	if ci := mentionContextInfo(msg, messageType); ci != nil {
		ci.MentionedJID = jids
	}
}
