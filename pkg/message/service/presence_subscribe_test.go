package message_service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

type fakePresenceClient struct {
	events      []string
	failSubOn   map[string]bool
	failPresent bool
}

func (f *fakePresenceClient) SendPresence(_ context.Context, state types.Presence) error {
	f.events = append(f.events, "presence:"+string(state))
	if f.failPresent {
		return errors.New("socket closed")
	}
	return nil
}

func (f *fakePresenceClient) SubscribePresence(_ context.Context, jid types.JID) error {
	f.events = append(f.events, "subscribe:"+jid.User)
	if f.failSubOn[jid.User] {
		return errors.New("no privacy token")
	}
	return nil
}

func TestPresenceTargetsAcceptsNumberAndNumbersWithoutDuplicates(t *testing.T) {
	valid, invalid, err := presenceTargets(&SubscribePresenceStruct{
		Number:  "5511999990001",
		Numbers: []string{"5511999990001", "+5511999990002@s.whatsapp.net", "5511999990003"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(invalid) != 0 {
		t.Fatalf("inválidos = %v", invalid)
	}

	var users []string
	for _, jid := range valid {
		users = append(users, jid.User)
		if jid.Server != types.DefaultUserServer {
			t.Errorf("servidor = %q", jid.Server)
		}
	}
	// O "+" sai (nó cru sem usync) e o repetido conta uma vez.
	want := []string{"5511999990001", "5511999990002", "5511999990003"}
	if !reflect.DeepEqual(users, want) {
		t.Fatalf("contatos = %v, esperado %v", users, want)
	}
}

func TestPresenceTargetsRejectsEmptyOversizedAndAllInvalid(t *testing.T) {
	var many []string
	for i := 0; i <= maxPresenceSubscriptions; i++ {
		many = append(many, fmt.Sprintf("55119999%05d", i))
	}

	cases := map[string]*SubscribePresenceStruct{
		"vazio":                {},
		"mais que o limite":    {Numbers: many},
		"nenhum número válido": {Number: "isto não é um telefone"},
		"só números em branco": {Numbers: []string{""}},
	}

	for name, data := range cases {
		if _, _, err := presenceTargets(data); !IsSubscribeRequestError(err) {
			t.Errorf("%s: erro = %v, esperado um erro de pedido inválido", name, err)
		}
	}
}

func TestPresenceTargetsReportsInvalidOnesAndKeepsTheValid(t *testing.T) {
	valid, invalid, err := presenceTargets(&SubscribePresenceStruct{Numbers: []string{"5511999990001", "lixo"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(valid) != 1 || len(invalid) != 1 || invalid[0].Number != "lixo" {
		t.Fatalf("válidos %v, inválidos %v", valid, invalid)
	}
}

// O WhatsApp só entrega a presença dos outros a quem está online: a instância
// anuncia presença antes de assinar.
func TestSubscribePresenceGoesOnlineBeforeSubscribing(t *testing.T) {
	client := &fakePresenceClient{}
	jids := []types.JID{types.NewJID("5511999990001", types.DefaultUserServer), types.NewJID("5511999990002", types.DefaultUserServer)}

	result, err := subscribePresence(context.Background(), client, jids, nil)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"presence:available", "subscribe:5511999990001", "subscribe:5511999990002"}
	if !reflect.DeepEqual(client.events, want) {
		t.Fatalf("chamadas = %v, esperado %v", client.events, want)
	}
	if len(result.Subscribed) != 2 || len(result.Failed) != 0 {
		t.Fatalf("resultado = %+v", result)
	}
}

func TestSubscribePresenceFailureOfOneContactDoesNotStopTheOthers(t *testing.T) {
	client := &fakePresenceClient{failSubOn: map[string]bool{"5511999990002": true}}
	jids := []types.JID{
		types.NewJID("5511999990001", types.DefaultUserServer),
		types.NewJID("5511999990002", types.DefaultUserServer),
		types.NewJID("5511999990003", types.DefaultUserServer),
	}
	invalid := []SubscribePresenceFailure{{Number: "lixo", Error: "invalid phone number"}}

	result, err := subscribePresence(context.Background(), client, jids, invalid)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Subscribed) != 2 {
		t.Errorf("assinados = %v", result.Subscribed)
	}
	if len(result.Failed) != 2 || result.Failed[0].Number != "lixo" || result.Failed[1].Number != "5511999990002@s.whatsapp.net" {
		t.Errorf("falhas = %+v", result.Failed)
	}
}

func TestSubscribePresenceFailsWhenItCannotGoOnline(t *testing.T) {
	client := &fakePresenceClient{failPresent: true}

	_, err := subscribePresence(context.Background(), client, []types.JID{types.NewJID("5511999990001", types.DefaultUserServer)}, nil)
	if err == nil {
		t.Fatal("esperava erro")
	}
	if IsSubscribeRequestError(err) {
		t.Error("falha de rede não é um pedido inválido (deve virar 500, não 400)")
	}
	for _, event := range client.events {
		if event == "subscribe:5511999990001" {
			t.Error("não deveria assinar sem conseguir ficar online")
		}
	}
}
