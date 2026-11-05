package send_service

import (
	"encoding/json"
	"testing"
)

// #189: a citação ia com QuotedMessage vazio e o cartão da resposta aparecia
// sem texto, sem poder ser tocado. O texto agora pode ir junto.
func TestQuotedContextInfoCarriesQuotedText(t *testing.T) {
	ci := quotedContextInfo(QuotedStruct{MessageID: "3EB0FAED", Participant: "5511999999999@s.whatsapp.net", Text: "mensagem original"})

	if ci.GetStanzaID() != "3EB0FAED" || ci.GetParticipant() != "5511999999999@s.whatsapp.net" {
		t.Fatalf("stanza/participant = %q / %q", ci.GetStanzaID(), ci.GetParticipant())
	}
	if got := ci.GetQuotedMessage().GetConversation(); got != "mensagem original" {
		t.Fatalf("quoted conversation = %q", got)
	}
}

// Sem texto, o comportamento continua igual ao de antes (citação vazia).
func TestQuotedContextInfoWithoutTextIsUnchanged(t *testing.T) {
	ci := quotedContextInfo(QuotedStruct{MessageID: "ID", Participant: "p"})
	if ci.QuotedMessage == nil || ci.GetQuotedMessage().Conversation == nil || ci.GetQuotedMessage().GetConversation() != "" {
		t.Fatalf("QuotedMessage = %#v, want an empty conversation like before", ci.QuotedMessage)
	}
}

func TestQuotedStructAcceptsTextFromJSON(t *testing.T) {
	var body struct {
		Quoted QuotedStruct `json:"quoted"`
	}
	if err := json.Unmarshal([]byte(`{"quoted":{"messageId":"ID","participant":"p","text":"oi"}}`), &body); err != nil {
		t.Fatal(err)
	}
	if body.Quoted.Text != "oi" || body.Quoted.MessageID != "ID" {
		t.Fatalf("quoted = %+v", body.Quoted)
	}

	// corpo antigo, sem text, continua válido
	if err := json.Unmarshal([]byte(`{"quoted":{"messageId":"ID","participant":"p"}}`), &body); err != nil {
		t.Fatal(err)
	}
}
