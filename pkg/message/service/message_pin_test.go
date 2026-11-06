package message_service

import (
	"errors"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
)

const (
	pinDirect = "5511999999999@s.whatsapp.net"
	pinGroup  = "120363000000000000@g.us"
)

var pinNow = time.Unix(1700000000, 0)

func TestPinMessageContent(t *testing.T) {
	chat, msg, err := buildPinMessage(&PinStruct{Number: pinDirect, Id: "M1", FromMe: true, Duration: pinFor24Hours}, true, pinNow)
	if err != nil {
		t.Fatal(err)
	}
	if chat.String() != pinDirect {
		t.Fatalf("destino %s", chat)
	}
	pin := msg.GetPinInChatMessage()
	if pin.GetType() != waE2E.PinInChatMessage_PIN_FOR_ALL {
		t.Fatalf("tipo %v", pin.GetType())
	}
	if pin.GetSenderTimestampMS() != pinNow.UnixMilli() {
		t.Fatalf("horário %d", pin.GetSenderTimestampMS())
	}
	if pin.GetKey().GetID() != "M1" || !pin.GetKey().GetFromMe() || pin.GetKey().GetRemoteJID() != pinDirect {
		t.Fatalf("chave %+v", pin.GetKey())
	}
	if msg.GetMessageContextInfo().GetMessageAddOnDurationInSecs() != pinFor24Hours {
		t.Fatalf("prazo %d", msg.GetMessageContextInfo().GetMessageAddOnDurationInSecs())
	}
}

func TestPinDefaultsToSevenDays(t *testing.T) {
	_, msg, err := buildPinMessage(&PinStruct{Number: pinDirect, Id: "M1"}, true, pinNow)
	if err != nil {
		t.Fatal(err)
	}
	if got := msg.GetMessageContextInfo().GetMessageAddOnDurationInSecs(); got != pinFor7Days {
		t.Fatalf("prazo %d", got)
	}
}

func TestUnpinHasNoDuration(t *testing.T) {
	_, msg, err := buildPinMessage(&PinStruct{Number: pinDirect, Id: "M1", Duration: 12345}, false, pinNow)
	if err != nil {
		t.Fatal(err)
	}
	if msg.GetPinInChatMessage().GetType() != waE2E.PinInChatMessage_UNPIN_FOR_ALL {
		t.Fatalf("tipo %v", msg.GetPinInChatMessage().GetType())
	}
	if got := msg.GetMessageContextInfo().GetMessageAddOnDurationInSecs(); got != 0 {
		t.Fatalf("prazo %d", got)
	}
}

func TestPinInGroupNeedsTheAuthor(t *testing.T) {
	_, msg, err := buildPinMessage(&PinStruct{Number: pinGroup, Id: "M1", Participant: "5511999999999"}, true, pinNow)
	if err != nil {
		t.Fatal(err)
	}
	if got := msg.GetPinInChatMessage().GetKey().GetParticipant(); got == "" || got[0] == '+' {
		t.Fatalf("autor %q", got)
	}

	if _, _, err := buildPinMessage(&PinStruct{Number: pinGroup, Id: "M1"}, true, pinNow); !errors.Is(err, ErrInvalidPin) {
		t.Fatalf("erro %v", err)
	}
	if _, _, err := buildPinMessage(&PinStruct{Number: pinGroup, Id: "M1", FromMe: true}, true, pinNow); err != nil {
		t.Fatalf("mensagem própria não precisa de autor: %v", err)
	}
}

func TestPinRefusesBadInput(t *testing.T) {
	bad := map[string]PinStruct{
		"sem destino":  {Id: "M1"},
		"sem mensagem": {Number: pinDirect},
		"prazo errado": {Number: pinDirect, Id: "M1", Duration: 3600},
	}
	for name, data := range bad {
		if _, _, err := buildPinMessage(&data, true, pinNow); !errors.Is(err, ErrInvalidPin) {
			t.Errorf("%s: erro %v", name, err)
		}
	}
}

func TestPinOfBareNumberHasNoPlusSign(t *testing.T) {
	chat, msg, err := buildPinMessage(&PinStruct{Number: "5511999999999", Id: "M1"}, true, pinNow)
	if err != nil {
		t.Fatal(err)
	}
	if chat.User[0] == '+' || msg.GetPinInChatMessage().GetKey().GetRemoteJID()[0] == '+' {
		t.Fatalf("destino %s, chave %s", chat, msg.GetPinInChatMessage().GetKey().GetRemoteJID())
	}
}
