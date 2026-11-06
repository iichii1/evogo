package send_service

import (
	"errors"
	"strings"
	"testing"

	"github.com/evolution-foundation/evolution-go/pkg/utils"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func TestContactsCardCarriesEveryContact(t *testing.T) {
	msg, err := buildContactsMessage(&ContactsStruct{Contacts: []utils.VCardStruct{
		{FullName: "Ana", Phone: "5511911111111"},
		{FullName: "Bia", Phone: "5511922222222", Organization: "ACME"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	card := msg.GetContactsArrayMessage()
	if card.GetDisplayName() != "2 contatos" || len(card.GetContacts()) != 2 {
		t.Fatalf("cartão %+v", card)
	}
	if card.GetContacts()[1].GetDisplayName() != "Bia" || !strings.Contains(card.GetContacts()[1].GetVcard(), "waid=5511922222222") {
		t.Fatalf("contato %+v", card.GetContacts()[1])
	}

	named, err := buildContactsMessage(&ContactsStruct{DisplayName: "Equipe", Contacts: []utils.VCardStruct{{FullName: "Ana", Phone: "1"}}})
	if err != nil {
		t.Fatal(err)
	}
	if named.GetContactsArrayMessage().GetDisplayName() != "Equipe" {
		t.Fatal("o nome informado foi ignorado")
	}
}

func TestContactsCardRefusesBadInput(t *testing.T) {
	tooMany := make([]utils.VCardStruct, maxContactsInCard+1)
	for i := range tooMany {
		tooMany[i] = utils.VCardStruct{FullName: "A", Phone: "1"}
	}
	bad := map[string][]utils.VCardStruct{
		"sem contatos":      nil,
		"demais":            tooMany,
		"sem nome":          {{Phone: "1"}},
		"sem telefone":      {{FullName: "A"}},
		"quebra de linha":   {{FullName: "A\nTEL:999", Phone: "1"}},
		"quebra na empresa": {{FullName: "A", Phone: "1", Organization: "B\r\nX"}},
	}
	for name, contacts := range bad {
		if _, err := buildContactsMessage(&ContactsStruct{Contacts: contacts}); !errors.Is(err, ErrInvalidSendInput) {
			t.Errorf("%s: erro %v", name, err)
		}
	}
}

func TestGroupInviteCard(t *testing.T) {
	msg, err := buildGroupInviteMessage(&GroupInviteCardStruct{
		GroupJID: "120363000000000000@g.us", Code: "C", GroupName: "Vendas", InviteExpiration: 1700000000, Caption: "Entre",
	})
	if err != nil {
		t.Fatal(err)
	}
	invite := msg.GetGroupInviteMessage()
	if invite.GetGroupJID() != "120363000000000000@g.us" || invite.GetInviteCode() != "C" || invite.GetGroupName() != "Vendas" ||
		invite.GetInviteExpiration() != 1700000000 || invite.GetCaption() != "Entre" {
		t.Fatalf("convite %+v", invite)
	}

	bad := map[string]GroupInviteCardStruct{
		"sem grupo":      {Code: "C", GroupName: "V"},
		"não é um grupo": {GroupJID: "5511@s.whatsapp.net", Code: "C", GroupName: "V"},
		"sem código":     {GroupJID: "1@g.us", GroupName: "V"},
		"sem nome":       {GroupJID: "1@g.us", Code: "C"},
	}
	for name, data := range bad {
		if _, err := buildGroupInviteMessage(&data); !errors.Is(err, ErrInvalidSendInput) {
			t.Errorf("%s: erro %v", name, err)
		}
	}
}

func TestStatusTextColorsAndFont(t *testing.T) {
	font := int32(waE2E.ExtendedTextMessage_COURIERPRIME_BOLD)
	msg, err := buildStatusTextMessage(&StatusTextStruct{Text: "Oi", BackgroundColor: "#112233", TextColor: "#80FFFFFF", Font: &font})
	if err != nil {
		t.Fatal(err)
	}
	text := msg.GetExtendedTextMessage()
	if text.GetText() != "Oi" {
		t.Fatalf("texto %q", text.GetText())
	}
	if text.GetBackgroundArgb() != 0xFF112233 {
		t.Fatalf("fundo %x: sem alfa a cor deve ficar opaca", text.GetBackgroundArgb())
	}
	if text.GetTextArgb() != 0x80FFFFFF {
		t.Fatalf("texto %x", text.GetTextArgb())
	}
	if text.GetFont() != waE2E.ExtendedTextMessage_COURIERPRIME_BOLD {
		t.Fatalf("fonte %v", text.GetFont())
	}
}

func TestStatusTextWithoutOptionsStaysPlain(t *testing.T) {
	msg, err := buildStatusTextMessage(&StatusTextStruct{Text: "Oi"})
	if err != nil {
		t.Fatal(err)
	}
	text := msg.GetExtendedTextMessage()
	if text.BackgroundArgb != nil || text.TextArgb != nil || text.Font != nil {
		t.Fatalf("campos a mais: %+v", text)
	}
}

func TestStatusTextRefusesBadOptions(t *testing.T) {
	font := int32(999)
	bad := map[string]*StatusTextStruct{
		"cor curta":    {Text: "a", BackgroundColor: "#123"},
		"cor inválida": {Text: "a", TextColor: "#GGGGGG"},
		"fonte":        {Text: "a", Font: &font},
	}
	for name, data := range bad {
		if _, err := buildStatusTextMessage(data); !errors.Is(err, ErrInvalidSendInput) {
			t.Errorf("%s: erro %v", name, err)
		}
	}
}

func TestNewMessagesHaveAContextForExpiration(t *testing.T) {
	messages := map[string]*waE2E.Message{
		"convite":  {GroupInviteMessage: &waE2E.GroupInviteMessage{ContextInfo: &waE2E.ContextInfo{}}},
		"telefone": {RequestPhoneNumberMessage: &waE2E.RequestPhoneNumberMessage{ContextInfo: &waE2E.ContextInfo{}}},
	}
	for name, msg := range messages {
		applyExpiration(msg, proto.Uint32(86400))
		if messageContextInfo(msg).GetExpiration() != 86400 {
			t.Errorf("%s: sem expiration", name)
		}
	}
}
