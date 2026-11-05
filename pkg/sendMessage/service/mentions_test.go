package send_service

import (
	"reflect"
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

var mentionJIDs = []string{"5511999990001@s.whatsapp.net", "5511999990002@s.whatsapp.net"}

// #114: documento com legenda vai embrulhado em DocumentWithCaptionMessage, e o
// código lia msg.DocumentMessage (nulo) para gravar as menções.
func TestSetMentionsOnDocumentWithCaption(t *testing.T) {
	doc := &waE2E.DocumentMessage{FileName: proto.String("a.pdf")}
	msg := &waE2E.Message{DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
		Message: &waE2E.Message{DocumentMessage: doc},
	}}

	setMentions(msg, "DocumentMessage", mentionJIDs)

	if !reflect.DeepEqual(doc.GetContextInfo().GetMentionedJID(), mentionJIDs) {
		t.Fatalf("menções = %v, esperado %v", doc.GetContextInfo().GetMentionedJID(), mentionJIDs)
	}
}

func TestSetMentionsOnEveryMessageType(t *testing.T) {
	cases := []struct {
		messageType string
		msg         *waE2E.Message
		contextInfo func(*waE2E.Message) *waE2E.ContextInfo
	}{
		{"ExtendedTextMessage", &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{}}, func(m *waE2E.Message) *waE2E.ContextInfo { return m.ExtendedTextMessage.ContextInfo }},
		{"ImageMessage", &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}, func(m *waE2E.Message) *waE2E.ContextInfo { return m.ImageMessage.ContextInfo }},
		{"VideoMessage", &waE2E.Message{VideoMessage: &waE2E.VideoMessage{}}, func(m *waE2E.Message) *waE2E.ContextInfo { return m.VideoMessage.ContextInfo }},
		{"PtvMessage", &waE2E.Message{PtvMessage: &waE2E.VideoMessage{}}, func(m *waE2E.Message) *waE2E.ContextInfo { return m.PtvMessage.ContextInfo }},
		{"AudioMessage", &waE2E.Message{AudioMessage: &waE2E.AudioMessage{}}, func(m *waE2E.Message) *waE2E.ContextInfo { return m.AudioMessage.ContextInfo }},
		{"DocumentMessage", &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{}}, func(m *waE2E.Message) *waE2E.ContextInfo { return m.DocumentMessage.ContextInfo }},
		{"PollCreationMessage", &waE2E.Message{PollCreationMessage: &waE2E.PollCreationMessage{}}, func(m *waE2E.Message) *waE2E.ContextInfo { return m.PollCreationMessage.ContextInfo }},
		{"StickerMessage", &waE2E.Message{StickerMessage: &waE2E.StickerMessage{}}, func(m *waE2E.Message) *waE2E.ContextInfo { return m.StickerMessage.ContextInfo }},
		{"LocationMessage", &waE2E.Message{LocationMessage: &waE2E.LocationMessage{}}, func(m *waE2E.Message) *waE2E.ContextInfo { return m.LocationMessage.ContextInfo }},
		{"ContactMessage", &waE2E.Message{ContactMessage: &waE2E.ContactMessage{}}, func(m *waE2E.Message) *waE2E.ContextInfo { return m.ContactMessage.ContextInfo }},
	}

	for _, c := range cases {
		setMentions(c.msg, c.messageType, mentionJIDs)
		if got := c.contextInfo(c.msg).GetMentionedJID(); !reflect.DeepEqual(got, mentionJIDs) {
			t.Errorf("%s: menções = %v, esperado %v", c.messageType, got, mentionJIDs)
		}
	}
}

// A citação já gravada no ContextInfo não pode se perder ao adicionar as menções.
func TestSetMentionsKeepsTheQuotedContext(t *testing.T) {
	msg := &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
		ContextInfo: &waE2E.ContextInfo{StanzaID: proto.String("ABC123")},
	}}

	setMentions(msg, "ImageMessage", mentionJIDs)

	ci := msg.ImageMessage.ContextInfo
	if ci.GetStanzaID() != "ABC123" || !reflect.DeepEqual(ci.GetMentionedJID(), mentionJIDs) {
		t.Fatalf("ContextInfo = %+v", ci)
	}
}

// Tipos sem menção e mensagens vazias não podem causar panic.
func TestSetMentionsIgnoresWhatDoesNotApply(t *testing.T) {
	msg := &waE2E.Message{InteractiveMessage: &waE2E.InteractiveMessage{}}
	setMentions(msg, "InteractiveMessage", mentionJIDs)
	if msg.InteractiveMessage.ContextInfo != nil {
		t.Error("carrossel/botões não aceitam menção, mas o ContextInfo foi alterado")
	}

	setMentions(&waE2E.Message{}, "DocumentMessage", mentionJIDs)
	setMentions(&waE2E.Message{}, "ImageMessage", mentionJIDs)
	setMentions(&waE2E.Message{}, "TipoQueNaoExiste", mentionJIDs)
}
