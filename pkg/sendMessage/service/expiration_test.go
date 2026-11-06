package send_service

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func TestApplyExpirationCoversEveryMessageWithContext(t *testing.T) {
	info := func() *waE2E.ContextInfo { return &waE2E.ContextInfo{} }
	messages := map[string]*waE2E.Message{
		"texto":     {ExtendedTextMessage: &waE2E.ExtendedTextMessage{ContextInfo: info()}},
		"imagem":    {ImageMessage: &waE2E.ImageMessage{ContextInfo: info()}},
		"vídeo":     {VideoMessage: &waE2E.VideoMessage{ContextInfo: info()}},
		"ptv":       {PtvMessage: &waE2E.VideoMessage{ContextInfo: info()}},
		"áudio":     {AudioMessage: &waE2E.AudioMessage{ContextInfo: info()}},
		"documento": {DocumentMessage: &waE2E.DocumentMessage{ContextInfo: info()}},
		"documento com legenda": {DocumentWithCaptionMessage: &waE2E.FutureProofMessage{Message: &waE2E.Message{
			DocumentMessage: &waE2E.DocumentMessage{ContextInfo: info()},
		}}},
		"enquete":     {PollCreationMessage: &waE2E.PollCreationMessage{ContextInfo: info()}},
		"figurinha":   {StickerMessage: &waE2E.StickerMessage{ContextInfo: info()}},
		"localização": {LocationMessage: &waE2E.LocationMessage{ContextInfo: info()}},
		"contato":     {ContactMessage: &waE2E.ContactMessage{ContextInfo: info()}},
		"contatos":    {ContactsArrayMessage: &waE2E.ContactsArrayMessage{ContextInfo: info()}},
	}
	for name, msg := range messages {
		applyExpiration(msg, proto.Uint32(604800))
		if got := messageContextInfo(msg).GetExpiration(); got != 604800 {
			t.Errorf("%s: expiration %d", name, got)
		}
	}
}

func TestApplyExpirationIgnoresWhatHasNoTimer(t *testing.T) {
	msg := &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{ContextInfo: &waE2E.ContextInfo{}}}

	applyExpiration(msg, nil)
	if msg.ExtendedTextMessage.ContextInfo.Expiration != nil {
		t.Fatal("sem valor, o campo fica vazio")
	}
	applyExpiration(msg, proto.Uint32(0))
	if msg.ExtendedTextMessage.ContextInfo.Expiration != nil {
		t.Fatal("zero é conversa sem prazo: o campo fica vazio")
	}

	// Formato sem contexto, como o de um botão, não pode quebrar.
	applyExpiration(&waE2E.Message{ButtonsMessage: &waE2E.ButtonsMessage{}}, proto.Uint32(86400))
	applyExpiration(&waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{}}, proto.Uint32(86400))
}
