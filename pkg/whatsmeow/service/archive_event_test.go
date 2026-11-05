package whatsmeow_service

import (
	"encoding/json"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// Regressão de #95/#101: o handler fazia postMap["data"].(map[string]interface{})
// sobre o *events.Archive bruto e o panic derrubava o processo inteiro.
func TestArchiveEventDataBuildsMapWithoutPanic(t *testing.T) {
	evt := &events.Archive{
		JID:          types.NewJID("5511999999999", types.DefaultUserServer),
		Timestamp:    time.Unix(1700000000, 0),
		Action:       &waSyncAction.ArchiveChatAction{Archived: proto.Bool(true)},
		FromFullSync: true,
	}

	data := archiveEventData(evt)

	for _, key := range []string{"JID", "Timestamp", "Action", "FromFullSync"} {
		if _, ok := data[key]; !ok {
			t.Fatalf("payload sem a chave %q: %v", key, data)
		}
	}
	if data["FromFullSync"] != true {
		t.Fatalf("FromFullSync = %v", data["FromFullSync"])
	}

	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("payload não serializa para o webhook: %v", err)
	}
	var back map[string]interface{}
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back["JID"] != "5511999999999@s.whatsapp.net" {
		t.Fatalf("JID serializado = %v", back["JID"])
	}
}
