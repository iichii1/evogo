package whatsmeow_service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

type fakeDecrypter struct {
	message *waE2E.Message
	err     error
	called  int
}

func (f *fakeDecrypter) DecryptSecretEncryptedMessage(context.Context, *events.Message) (*waE2E.Message, error) {
	f.called++
	return f.message, f.err
}

func secretEdit(encType waE2E.SecretEncryptedMessage_SecretEncType) *events.Message {
	return &events.Message{Message: &waE2E.Message{
		SecretEncryptedMessage: &waE2E.SecretEncryptedMessage{
			SecretEncType:    encType.Enum(),
			TargetMessageKey: &waCommon.MessageKey{ID: proto.String("ORIGINAL-ID")},
			EncPayload:       []byte("cifrado"),
			EncIV:            []byte("iv"),
		},
	}}
}

// #92 e #62: o webhook de uma edição trazia só o envelope cifrado.
func TestBuildMessageActionDecryptsAnEdit(t *testing.T) {
	decrypter := &fakeDecrypter{message: &waE2E.Message{Conversation: proto.String("Edited message")}}

	action := buildMessageAction(context.Background(), decrypter, secretEdit(waE2E.SecretEncryptedMessage_MESSAGE_EDIT))

	if action == nil || !action.isEdit() {
		t.Fatalf("esperava uma edição, veio %v", action)
	}
	if action["targetMessageId"] != "ORIGINAL-ID" || action["decrypted"] != true {
		t.Fatalf("ação = %v", action)
	}

	// O que vai para o webhook é JSON: o texto novo precisa estar lá.
	raw, err := json.Marshal(action)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"conversation":"Edited message"`) {
		t.Fatalf("o texto novo não aparece no JSON do webhook: %s", raw)
	}
}

func TestBuildMessageActionKeepsTheEventWhenDecryptionFails(t *testing.T) {
	decrypter := &fakeDecrypter{err: errors.New("no message secret found")}

	action := buildMessageAction(context.Background(), decrypter, secretEdit(waE2E.SecretEncryptedMessage_MESSAGE_EDIT))

	if action == nil || !action.isEdit() {
		t.Fatalf("esperava uma edição, veio %v", action)
	}
	if action["decrypted"] != false || action["targetMessageId"] != "ORIGINAL-ID" {
		t.Fatalf("ação = %v", action)
	}
	if _, has := action["message"]; has {
		t.Error("sem decifrar, não pode haver conteúdo")
	}
	if !strings.Contains(action["error"].(string), "no message secret") {
		t.Errorf("o motivo da falha deveria acompanhar a ação: %v", action["error"])
	}
}

func TestBuildMessageActionNamesTheOtherSecretTypes(t *testing.T) {
	cases := map[waE2E.SecretEncryptedMessage_SecretEncType]string{
		waE2E.SecretEncryptedMessage_EVENT_EDIT:      "eventEdit",
		waE2E.SecretEncryptedMessage_POLL_EDIT:       "pollEdit",
		waE2E.SecretEncryptedMessage_POLL_ADD_OPTION: "pollAddOption",
	}
	for encType, want := range cases {
		action := buildMessageAction(context.Background(), &fakeDecrypter{message: &waE2E.Message{}}, secretEdit(encType))
		if action == nil || action["type"] != want {
			t.Errorf("%v: tipo = %v, esperado %q", encType, action["type"], want)
		}
		if action.isEdit() {
			t.Errorf("%v não é uma edição de mensagem (IsEdit não deve virar true)", encType)
		}
	}
}

// #92: a exclusão só dizia `protocolMessage.type: 0`.
func TestBuildMessageActionMarksARevoke(t *testing.T) {
	evt := &events.Message{Message: &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{
		Type: waE2E.ProtocolMessage_REVOKE.Enum(),
		Key:  &waCommon.MessageKey{ID: proto.String("APAGADA-ID")},
	}}}
	decrypter := &fakeDecrypter{}

	action := buildMessageAction(context.Background(), decrypter, evt)

	if action == nil || action["type"] != "revoke" || action["targetMessageId"] != "APAGADA-ID" {
		t.Fatalf("ação = %v", action)
	}
	if action.isEdit() {
		t.Error("exclusão não é edição")
	}
	if decrypter.called != 0 {
		t.Error("exclusão não precisa decifrar nada")
	}
}

func TestBuildMessageActionIgnoresOrdinaryMessages(t *testing.T) {
	cases := []*events.Message{
		nil,
		{},
		{Message: &waE2E.Message{Conversation: proto.String("oi")}},
		// Protocol message que não é exclusão (por exemplo, troca do timer de mensagens temporárias).
		{Message: &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{Type: waE2E.ProtocolMessage_EPHEMERAL_SETTING.Enum()}}},
		// Revoke sem a chave da mensagem alvo: não há o que apontar.
		{Message: &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{Type: waE2E.ProtocolMessage_REVOKE.Enum()}}},
	}
	for i, evt := range cases {
		if action := buildMessageAction(context.Background(), &fakeDecrypter{}, evt); action != nil {
			t.Errorf("caso %d: esperava nil, veio %v", i, action)
		}
	}
}

func TestBuildMessageActionWithoutAClientStillReportsTheEdit(t *testing.T) {
	action := buildMessageAction(context.Background(), nil, secretEdit(waE2E.SecretEncryptedMessage_MESSAGE_EDIT))
	if action == nil || !action.isEdit() || action["decrypted"] != false {
		t.Fatalf("ação = %v", action)
	}
}
