package send_service

import (
	"context"
	"path/filepath"
	"testing"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waAdv"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	_ "modernc.org/sqlite"
)

// newOfflineClient cria um cliente whatsmeow com store SQLite e identidade
// fictícia, sem conectar a lugar nenhum.
func newOfflineClient(t *testing.T) (*whatsmeow.Client, types.JID, types.JID) {
	t.Helper()
	ctx := context.Background()

	dsn := "file:" + filepath.Join(t.TempDir(), "wa.db") + "?_pragma=foreign_keys(1)"
	container, err := sqlstore.New(ctx, "sqlite", dsn, nil)
	if err != nil {
		t.Fatal(err)
	}

	ownPN := types.NewADJID("5511000000000", 0, 12)
	ownLID := types.NewJID("111111111111111", types.HiddenUserServer)

	device := container.NewDevice()
	device.ID = &ownPN
	device.LID = ownLID
	device.Account = &waAdv.ADVSignedDeviceIdentity{
		Details:             []byte{1},
		AccountSignature:    make([]byte, 64),
		AccountSignatureKey: make([]byte, 32),
		DeviceSignature:     make([]byte, 64),
	}
	if err := device.Save(ctx); err != nil {
		t.Fatal(err)
	}

	client := whatsmeow.NewClient(device, nil)
	return client, ownPN, ownLID
}

func TestPollVoteUsesStoredSecretOfOwnPoll(t *testing.T) {
	ctx := context.Background()
	client, _, ownLID := newOfflineClient(t)

	contactPN := types.NewJID("5511999999999", types.DefaultUserServer)
	contactLID := types.NewJID("222222222222222", types.HiddenUserServer)
	if err := client.Store.LIDs.PutLIDMapping(ctx, contactLID, contactPN); err != nil {
		t.Fatal(err)
	}

	// É assim que o whatsmeow grava a chave quando envia uma enquete para um
	// contato: chat e autor em LID (send.go troca o destino PN pelo LID).
	pollID := types.MessageID("3EB0POLL0000000001")
	if err := client.Store.MsgSecrets.PutMessageSecret(ctx, contactLID, ownLID, pollID, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}

	data := &PollVoteStruct{Number: "5511999999999", PollMessageID: string(pollID), FromMe: true, SelectedOptions: []string{"Sim"}}
	chat, sender, err := pollVoteTarget(data, *client.Store.ID)
	if err != nil {
		t.Fatal(err)
	}
	sender = resolvePollSender(ctx, client, chat, sender)
	if sender != ownLID {
		t.Fatalf("sender = %s, want own LID %s", sender, ownLID)
	}

	msg, err := client.BuildPollVote(ctx, &types.MessageInfo{
		MessageSource: types.MessageSource{Chat: chat, Sender: sender, IsFromMe: true},
		ID:            pollID,
	}, data.SelectedOptions)
	if err != nil {
		t.Fatalf("BuildPollVote: %v", err)
	}
	if msg.GetPollUpdateMessage().GetVote().GetEncPayload() == nil {
		t.Fatal("vote payload is empty")
	}
}

func TestPollVoteUsesStoredSecretOfReceivedPoll(t *testing.T) {
	ctx := context.Background()
	client, _, _ := newOfflineClient(t)

	contactPN := types.NewJID("5511999999999", types.DefaultUserServer)
	contactLID := types.NewJID("222222222222222", types.HiddenUserServer)
	if err := client.Store.LIDs.PutLIDMapping(ctx, contactLID, contactPN); err != nil {
		t.Fatal(err)
	}

	// Enquete recebida de um contato numa conversa em LID.
	pollID := types.MessageID("3EB0POLL0000000002")
	if err := client.Store.MsgSecrets.PutMessageSecret(ctx, contactLID, contactLID, pollID, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}

	// O usuário da API só sabe o telefone do contato.
	data := &PollVoteStruct{Number: "5511999999999", PollMessageID: string(pollID), SelectedOptions: []string{"Não"}}
	chat, sender, err := pollVoteTarget(data, *client.Store.ID)
	if err != nil {
		t.Fatal(err)
	}
	sender = resolvePollSender(ctx, client, chat, sender)
	if sender != contactLID {
		t.Fatalf("sender = %s, want contact LID %s", sender, contactLID)
	}

	if _, err := client.BuildPollVote(ctx, &types.MessageInfo{
		MessageSource: types.MessageSource{Chat: chat, Sender: sender},
		ID:            pollID,
	}, data.SelectedOptions); err != nil {
		t.Fatalf("BuildPollVote: %v", err)
	}
}

func TestPollVoteFailsForUnknownPoll(t *testing.T) {
	ctx := context.Background()
	client, _, _ := newOfflineClient(t)

	data := &PollVoteStruct{Number: "5511999999999", PollMessageID: "UNKNOWN", SelectedOptions: []string{"Sim"}}
	chat, sender, err := pollVoteTarget(data, *client.Store.ID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.BuildPollVote(ctx, &types.MessageInfo{
		MessageSource: types.MessageSource{Chat: chat, Sender: sender},
		ID:            "UNKNOWN",
	}, data.SelectedOptions)
	if err == nil {
		t.Fatal("expected an error for a poll whose secret is not stored")
	}
}
