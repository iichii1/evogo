package user_service

import (
	"errors"
	"testing"
)

func TestParseBusinessJID(t *testing.T) {
	for _, bad := range []string{"", "@@"} {
		if _, err := parseBusinessJID(bad); !errors.Is(err, ErrInvalidUserRequest) {
			t.Errorf("%q: erro %v", bad, err)
		}
	}
	jid, err := parseBusinessJID("5511999999999@s.whatsapp.net")
	if err != nil {
		t.Fatal(err)
	}
	if jid.User != "5511999999999" {
		t.Fatalf("jid %s", jid)
	}
}

// A validação vem antes de procurar o cliente: um valor errado responde 400 mesmo
// com a instância fora do ar.
func TestSetDefaultDisappearingRefusesAnUnknownTimerFirst(t *testing.T) {
	service := &userService{}
	for _, timer := range []string{"", "1h", "banana"} {
		err := service.SetDefaultDisappearing(&DefaultDisappearingStruct{Timer: timer}, nil)
		if !errors.Is(err, ErrInvalidUserRequest) {
			t.Errorf("%q: erro %v", timer, err)
		}
	}
}

func TestBusinessJIDHasNoPlusSign(t *testing.T) {
	jid, err := parseBusinessJID("5511999999999")
	if err != nil {
		t.Fatal(err)
	}
	if jid.User == "" || jid.User[0] == '+' {
		t.Fatalf("jid %q", jid.String())
	}
}

// Num contato comum, a whatsmeow responde com erro (ensaio de 2026-11-06).
func TestAContactWithoutBusinessProfileIsNotAFailure(t *testing.T) {
	if !isNoBusinessProfile(errors.New("missing jid in business profile")) {
		t.Error("contato sem conta comercial deveria ser reconhecido")
	}
	if isNoBusinessProfile(nil) || isNoBusinessProfile(errors.New("info query timed out")) {
		t.Error("outras falhas continuam sendo falhas")
	}
}
