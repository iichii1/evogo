package group_service

import (
	"errors"
	"testing"
)

func TestGroupInviteParseRequiresEveryField(t *testing.T) {
	const group = "120363000000000000@g.us"
	const inviter = "5511999999999@s.whatsapp.net"

	bad := map[string]GroupInviteStruct{
		"sem grupo":        {Inviter: inviter, Code: "C"},
		"grupo inválido":   {GroupJID: "@@", Inviter: inviter, Code: "C"},
		"não é um grupo":   {GroupJID: inviter, Inviter: inviter, Code: "C"},
		"sem quem convida": {GroupJID: group, Code: "C"},
		"sem código":       {GroupJID: group, Inviter: inviter},
	}
	for name, data := range bad {
		if _, _, err := data.parse(); !errors.Is(err, ErrInvalidGroupInvite) {
			t.Errorf("%s: erro %v", name, err)
		}
	}

	good := GroupInviteStruct{GroupJID: group, Inviter: inviter, Code: "C", Expiration: 1700000000}
	parsedGroup, parsedInviter, err := good.parse()
	if err != nil {
		t.Fatal(err)
	}
	if parsedGroup.String() != group || parsedInviter.String() != inviter {
		t.Fatalf("grupo %s, convidou %s", parsedGroup, parsedInviter)
	}
}

func TestRequestParticipantsHaveNoPlusSign(t *testing.T) {
	jids, err := parseRequestParticipants([]string{"5511999999999", "5511888888888@s.whatsapp.net"})
	if err != nil {
		t.Fatal(err)
	}
	for _, jid := range jids {
		if jid.User == "" || jid.User[0] == '+' {
			t.Fatalf("jid %q", jid.String())
		}
	}
	if _, err := parseRequestParticipants([]string{""}); err == nil {
		t.Fatal("participante vazio precisa dar erro")
	}
}

func TestInviterHasNoPlusSign(t *testing.T) {
	_, inviter, err := (&GroupInviteStruct{GroupJID: "1@g.us", Inviter: "5511999999999", Code: "C"}).parse()
	if err != nil {
		t.Fatal(err)
	}
	if inviter.User == "" || inviter.User[0] == '+' {
		t.Fatalf("inviter %q", inviter.String())
	}
}
