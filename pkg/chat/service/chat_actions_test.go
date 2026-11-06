package chat_service

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
)

const (
	directChat = "5511999999999@s.whatsapp.net"
	groupChat  = "120363000000000000@g.us"
)

func TestMarkChatUsesTheRequestedStateAndLastMessage(t *testing.T) {
	patch, err := buildMarkChatPatch(&ChatRangeStruct{
		Chat: directChat, LastMessageId: "M1", LastMessageTimestamp: 1700000000, LastMessageFromMe: true,
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if patch.Type != appstate.WAPatchRegularLow || len(patch.Mutations) != 1 {
		t.Fatalf("patch %+v", patch)
	}
	mutation := patch.Mutations[0]
	if mutation.Index[0] != appstate.IndexMarkChatAsRead || mutation.Index[1] != directChat {
		t.Fatalf("índice %v", mutation.Index)
	}
	action := mutation.Value.GetMarkChatAsReadAction()
	if action.GetRead() {
		t.Fatal("markunread gravou como lida")
	}
	messageRange := action.GetMessageRange()
	if messageRange.GetLastMessageTimestamp() != 1700000000 {
		t.Fatalf("horário %d", messageRange.GetLastMessageTimestamp())
	}
	key := messageRange.GetMessages()[0].GetKey()
	if key.GetID() != "M1" || !key.GetFromMe() || key.GetRemoteJID() != directChat || key.Participant != nil {
		t.Fatalf("chave %+v", key)
	}

	read, err := buildMarkChatPatch(&ChatRangeStruct{Chat: directChat}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !read.Mutations[0].Value.GetMarkChatAsReadAction().GetRead() {
		t.Fatal("markread gravou como não lida")
	}
}

func TestGroupLastMessageNeedsItsAuthor(t *testing.T) {
	_, err := buildMarkChatPatch(&ChatRangeStruct{Chat: groupChat, LastMessageId: "M1"}, true)
	if !errors.Is(err, ErrInvalidChatAction) {
		t.Fatalf("erro %v", err)
	}

	patch, err := buildMarkChatPatch(&ChatRangeStruct{Chat: groupChat, LastMessageId: "M1", LastMessageSender: directChat}, true)
	if err != nil {
		t.Fatal(err)
	}
	key := patch.Mutations[0].Value.GetMarkChatAsReadAction().GetMessageRange().GetMessages()[0].GetKey()
	if key.GetParticipant() != directChat {
		t.Fatalf("participante %q", key.GetParticipant())
	}

	if _, err := buildMarkChatPatch(&ChatRangeStruct{Chat: groupChat, LastMessageId: "M1", LastMessageFromMe: true}, true); err != nil {
		t.Fatalf("mensagem própria não precisa de autor: %v", err)
	}
}

func TestDeleteChatCarriesDeleteMedia(t *testing.T) {
	patch, err := buildDeleteChatPatch(&ChatRangeStruct{Chat: directChat, DeleteMedia: true})
	if err != nil {
		t.Fatal(err)
	}
	mutation := patch.Mutations[0]
	if mutation.Index[0] != appstate.IndexDeleteChat || mutation.Index[1] != directChat || mutation.Index[2] != "1" {
		t.Fatalf("índice %v", mutation.Index)
	}

	if _, err := buildDeleteChatPatch(&ChatRangeStruct{Chat: ""}); !errors.Is(err, ErrInvalidChatAction) {
		t.Fatalf("erro %v", err)
	}
}

func TestStarUsesTheAuthorOnlyInGroups(t *testing.T) {
	direct, err := buildStarPatch(&StarStruct{Chat: directChat, Id: "M1"}, true)
	if err != nil {
		t.Fatal(err)
	}
	index := direct.Mutations[0].Index
	if index[0] != appstate.IndexStar || index[1] != directChat || index[2] != "M1" || index[3] != "0" || index[4] != "0" {
		t.Fatalf("índice %v", index)
	}
	if !direct.Mutations[0].Value.GetStarAction().GetStarred() {
		t.Fatal("deveria favoritar")
	}

	group, err := buildStarPatch(&StarStruct{Chat: groupChat, Id: "M1", Sender: directChat}, false)
	if err != nil {
		t.Fatal(err)
	}
	if group.Mutations[0].Index[4] != directChat {
		t.Fatalf("índice %v", group.Mutations[0].Index)
	}
	if group.Mutations[0].Value.GetStarAction().GetStarred() {
		t.Fatal("deveria desfavoritar")
	}

	bad := []*StarStruct{
		{Chat: directChat},
		{Id: "M1"},
		{Chat: groupChat, Id: "M1"},
	}
	for _, data := range bad {
		if _, err := buildStarPatch(data, true); !errors.Is(err, ErrInvalidChatAction) {
			t.Errorf("%+v: erro %v", data, err)
		}
	}
}

func TestDisappearingTimerAcceptsOnlyWhatsAppValues(t *testing.T) {
	good := map[string]time.Duration{
		"off": whatsmeow.DisappearingTimerOff,
		"24h": whatsmeow.DisappearingTimer24Hours,
		"7d":  whatsmeow.DisappearingTimer7Days,
		"90d": whatsmeow.DisappearingTimer90Days,
	}
	for value, want := range good {
		got, err := ParseDisappearingTimer(value)
		if err != nil || got != want {
			t.Errorf("%s: %v, %v", value, got, err)
		}
	}
	for _, value := range []string{"", "1h", "30d", "banana"} {
		if _, err := ParseDisappearingTimer(value); !errors.Is(err, ErrInvalidChatAction) {
			t.Errorf("%q: erro %v", value, err)
		}
	}
}

// Um número solto ganha um "+" em utils.ParseJID, que vai parar no índice do app
// state e faz a ação não encontrar a conversa nos outros aparelhos.
func TestBareNumberHasNoPlusSignInTheAppState(t *testing.T) {
	jid, ok := canonicalJID("5511999999999")
	if !ok || jid.User == "" || jid.User[0] == '+' {
		t.Fatalf("jid %q, ok %v", jid.String(), ok)
	}

	patch, err := buildMarkChatPatch(&ChatRangeStruct{Chat: "5511999999999"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if index := patch.Mutations[0].Index[1]; index[0] == '+' {
		t.Fatalf("índice %q", index)
	}

	if _, ok := canonicalJID(""); ok {
		t.Fatal("vazio não é um JID")
	}
}

func TestOldChatRoutesUseTheCanonicalJID(t *testing.T) {
	src, err := os.ReadFile("chat_service.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), "utils.ParseJID") {
		t.Fatal("chat_service.go voltou a usar utils.ParseJID, que deixa o + no índice do app state")
	}
}
