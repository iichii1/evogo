package whatsmeow_service

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"github.com/evolution-foundation/evolution-go/pkg/internal/event_types"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// Todo evento repassado, com o nome e o grupo esperados.
var forwardedCases = []struct {
	evt   interface{}
	name  string
	group string
}{
	{&events.MarkChatAsRead{Action: &waSyncAction.MarkChatAsReadAction{Read: proto.Bool(true)}}, "MarkChatAsRead", event_types.CHAT_SYNC},
	{&events.DeleteChat{}, "DeleteChat", event_types.CHAT_SYNC},
	{&events.ClearChat{}, "ClearChat", event_types.CHAT_SYNC},
	{&events.DeleteForMe{}, "DeleteForMe", event_types.CHAT_SYNC},
	{&events.Mute{}, "Mute", event_types.CHAT_SYNC},
	{&events.Pin{}, "Pin", event_types.CHAT_SYNC},
	{&events.Star{}, "Star", event_types.CHAT_SYNC},
	{&events.UserStatusMute{}, "UserStatusMute", event_types.CHAT_SYNC},
	{&events.Blocklist{}, "Blocklist", event_types.CHAT_SYNC},
	{&events.PrivacySettings{}, "PrivacySettings", event_types.CHAT_SYNC},
	{&events.UnarchiveChatsSetting{}, "UnarchiveChatsSetting", event_types.CHAT_SYNC},
	{&events.CallReject{}, "CallReject", event_types.CALL},
	{&events.CallPreAccept{}, "CallPreAccept", event_types.CALL},
	{&events.CallTransport{}, "CallTransport", event_types.CALL},
	{&events.BusinessName{}, "BusinessName", event_types.CONTACT},
	{&events.ClientOutdated{}, "ClientOutdated", event_types.CONNECTION},
	{&events.KeepAliveTimeout{}, "KeepAliveTimeout", event_types.CONNECTION},
	{&events.KeepAliveRestored{}, "KeepAliveRestored", event_types.CONNECTION},
	{&events.OfflineSyncPreview{}, "OfflineSyncPreview", event_types.CONNECTION},
	{&events.StreamError{Code: "515"}, "StreamError", event_types.CONNECTION},
	{&events.AppStateSyncError{Name: appstate.WAPatchRegular, Error: errors.New("falhou")}, "AppStateSyncError", event_types.CONNECTION},
	{&events.CATRefreshError{Error: errors.New("falhou")}, "CATRefreshError", event_types.CONNECTION},
}

func TestForwardedEventsHaveANameAGroupAndValidJSON(t *testing.T) {
	for _, c := range forwardedCases {
		name, data := forwardedEvent(c.evt)
		if name != c.name {
			t.Errorf("%T: nome %q, esperava %q", c.evt, name, c.name)
		}
		if got := globalEventGroup(name); got != c.group {
			t.Errorf("%s: grupo %q, esperava %q", name, got, c.group)
		}
		if _, err := json.Marshal(data); err != nil {
			t.Errorf("%s: não vira JSON: %v", name, err)
		}
	}
}

func TestEventNotForwardedStaysOut(t *testing.T) {
	if name, data := forwardedEvent(&events.AppState{}); name != "" || data != nil {
		t.Fatalf("evento sem repasse virou %q", name)
	}
}

func TestOfflineSyncCompletedAndUndecryptableHaveAGroup(t *testing.T) {
	if got := globalEventGroup("OfflineSyncCompleted"); got != event_types.CONNECTION {
		t.Fatalf("OfflineSyncCompleted: grupo %q", got)
	}
	if got := globalEventGroup("UndecryptableMessage"); got != event_types.MESSAGE {
		t.Fatalf("UndecryptableMessage: grupo %q", got)
	}
}

// Um `error` vira {} em JSON, e o nó bruto do StreamError traz o protocolo inteiro.
func TestForwardedEventsCarryTheErrorTextAndNotTheRawNode(t *testing.T) {
	_, syncData := forwardedEvent(&events.AppStateSyncError{Name: appstate.WAPatchRegular, Error: errors.New("LTHash mismatch"), FullSync: true})
	syncJSON, err := json.Marshal(syncData)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(syncJSON), "LTHash mismatch") {
		t.Fatalf("o texto do erro sumiu: %s", syncJSON)
	}

	secret := "5511999999999"
	_, streamData := forwardedEvent(&events.StreamError{Code: "515", Raw: &waBinary.Node{Tag: "stream:error", Attrs: waBinary.Attrs{"jid": secret}}})
	streamJSON, err := json.Marshal(streamData)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(streamJSON), secret) {
		t.Fatalf("o nó bruto vazou para o webhook: %s", streamJSON)
	}
	if !strings.Contains(string(streamJSON), "515") {
		t.Fatalf("o código sumiu: %s", streamJSON)
	}
}

func webhookService(t *testing.T, producer *recordingProducer) *whatsmeowService {
	t.Helper()
	return &whatsmeowService{
		config:          &config.Config{},
		loggerWrapper:   logger_wrapper.NewLoggerManager(&config.Config{LogDirectory: t.TempDir()}),
		webhookProducer: producer,
	}
}

func TestCallWebhookDeliversNewEventsByGroup(t *testing.T) {
	cases := []struct {
		name   string
		event  string
		events string
		want   int
	}{
		{"CHAT_SYNC recebe a leitura feita no celular", "MarkChatAsRead", "CHAT_SYNC", 1},
		{"sem CHAT_SYNC não recebe", "MarkChatAsRead", "MESSAGE,CONNECTION", 0},
		{"Archive continua em CHAT_PRESENCE", "Archive", "CHAT_PRESENCE", 1},
		{"Archive também em CHAT_SYNC", "Archive", "CHAT_SYNC", 1},
		{"ChatPresence não entra em CHAT_SYNC", "ChatPresence", "CHAT_SYNC", 0},
		{"saúde da conexão em CONNECTION", "StreamError", "CONNECTION", 1},
		{"saúde da conexão fora de MESSAGE", "StreamError", "MESSAGE", 0},
		{"OfflineSyncCompleted em CONNECTION", "OfflineSyncCompleted", "CONNECTION", 1},
		{"não decifrada em MESSAGE", "UndecryptableMessage", "MESSAGE", 1},
		{"não decifrada fora de CONNECTION", "UndecryptableMessage", "CONNECTION", 0},
		{"chamada rejeitada em CALL", "CallReject", "CALL", 1},
		{"evento desconhecido não sai", "AlgoNovo", "MESSAGE,CHAT_SYNC,CONNECTION", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := &recordingProducer{}
			payload := []byte(`{"event":"` + c.event + `","data":{}}`)
			webhookService(t, p).CallWebhook(&instance_model.Instance{Id: "i1", Webhook: "http://hook", Events: c.events}, "i1."+strings.ToLower(c.event), payload)
			if p.count() != c.want {
				t.Fatalf("chegou %d vezes, esperava %d", p.count(), c.want)
			}
		})
	}
}

func TestChatSyncIsPartOfAllEventTypes(t *testing.T) {
	found := false
	for _, e := range event_types.AllEventTypes {
		found = found || e == event_types.CHAT_SYNC
	}
	if !found || !event_types.IsEventType(event_types.CHAT_SYNC) {
		t.Fatal("CHAT_SYNC deve ser válido e fazer parte de AllEventTypes")
	}
}

func TestConfigureClientAsksThePhoneForUndecryptableMessages(t *testing.T) {
	client := whatsmeow.NewClient(&store.Device{}, nil)
	configureClient(client)
	if !client.AutomaticMessageRerequestFromPhone {
		t.Fatal("a mensagem que não decifra precisa ser pedida ao celular")
	}
	if client.EnableAutoReconnect {
		t.Fatal("a reconexão continua sendo feita por este serviço, com backoff")
	}
	if !client.AutoTrustIdentity {
		t.Fatal("AutoTrustIdentity deixou de ser aplicado")
	}
}

func TestMediaRetryWaitersDeliverOnlyToTheSameInstanceAndMessage(t *testing.T) {
	w := newMediaRetryWaiters()
	first, cancelFirst := w.wait("i1", "M")
	defer cancelFirst()
	second, cancelSecond := w.wait("i1", "M")
	defer cancelSecond()
	otherInstance, cancelOtherInstance := w.wait("i2", "M")
	defer cancelOtherInstance()
	otherMessage, cancelOtherMessage := w.wait("i1", "N")
	defer cancelOtherMessage()

	evt := &events.MediaRetry{MessageID: types.MessageID("M")}
	w.deliver("i1", evt)

	for name, ch := range map[string]<-chan *events.MediaRetry{"primeiro": first, "segundo": second} {
		select {
		case got := <-ch:
			if got != evt {
				t.Errorf("%s recebeu outro evento", name)
			}
		default:
			t.Errorf("%s não recebeu", name)
		}
	}
	for name, ch := range map[string]<-chan *events.MediaRetry{"outra instância": otherInstance, "outra mensagem": otherMessage} {
		select {
		case <-ch:
			t.Errorf("%s recebeu a resposta", name)
		default:
		}
	}
}

func TestMediaRetryWaitersNeverBlockAndForgetCancelled(t *testing.T) {
	w := newMediaRetryWaiters()
	_, cancel := w.wait("i1", "M")
	evt := &events.MediaRetry{MessageID: "M"}
	w.deliver("i1", evt)
	w.deliver("i1", evt) // o canal já está cheio: não pode travar a goroutine de eventos
	cancel()
	cancel() // cancelar de novo não pode quebrar
	if len(w.waiters) != 0 {
		t.Fatalf("sobraram %d registros", len(w.waiters))
	}
	w.deliver("i1", evt) // sem ninguém esperando
}
