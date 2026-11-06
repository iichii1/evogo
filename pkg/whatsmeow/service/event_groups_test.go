package whatsmeow_service

import (
	"os"
	"regexp"
	"sort"
	"sync"
	"testing"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"github.com/evolution-foundation/evolution-go/pkg/internal/event_types"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
)

func TestGlobalEventGroup(t *testing.T) {
	cases := map[string]string{
		"Message":                 event_types.MESSAGE,
		"SendMessage":             event_types.SEND_MESSAGE,
		"Receipt":                 event_types.READ_RECEIPT,
		"Archive":                 event_types.CHAT_PRESENCE,
		"AccountReachoutTimelock": event_types.CONNECTION,
		"Picture":                 event_types.PICTURE,      // #193
		"UserAbout":               event_types.USER_ABOUT,   // #193
		"ButtonClick":             event_types.BUTTON_CLICK, // #193
		"PasskeyRequest":          event_types.PASSKEY,      // #105
		"PasskeyConfirmation":     event_types.PASSKEY,
		"PasskeyError":            event_types.PASSKEY,
		"SomethingNew":            "",
	}
	for evt, want := range cases {
		if got := globalEventGroup(evt); got != want {
			t.Errorf("globalEventGroup(%q) = %q, want %q", evt, got, want)
		}
	}
}

// Eventos que o handler emite de propósito sem pertencer a nenhum grupo.
var ungroupedEvents = map[string]bool{}

// Trava o problema de fundo de #105 e #193: um evento novo emitido em
// whatsmeow.go sem grupo, ou um grupo sem nenhum evento, era descartado em
// silêncio. Agora o teste falha e obriga a decidir.
func TestEveryEmittedEventHasAGroup(t *testing.T) {
	src, err := os.ReadFile("whatsmeow.go")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`(?:postMap\["event"\]\s*=\s*|"event":\s*)"(\w+)"`)
	emitted := map[string]bool{"SendMessage": true} // emitido por send_service.go
	for _, c := range forwardedCases {
		emitted[c.name] = true // emitidos por forwardedEvent
	}
	for _, m := range re.FindAllSubmatch(src, -1) {
		emitted[string(m[1])] = true
	}
	if len(emitted) < 20 {
		t.Fatalf("achei só %d eventos em whatsmeow.go, a expressão regular quebrou", len(emitted))
	}

	covered := map[string]bool{}
	var names []string
	for name := range emitted {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		group := globalEventGroup(name)
		if group == "" {
			if !ungroupedEvents[name] {
				t.Errorf("o evento %q é emitido mas não pertence a nenhum grupo em globalEventGroup", name)
			}
			continue
		}
		if !event_types.IsEventType(group) {
			t.Errorf("o grupo %q do evento %q não é um tipo de evento válido", group, name)
		}
		covered[group] = true
	}

	for _, group := range event_types.AllEventTypes {
		if !covered[group] {
			t.Errorf("nenhum evento emitido pertence ao grupo %q", group)
		}
	}
}

// "ALL" é expandido para AllEventTypes ao conectar; PASSKEY precisa estar lá
// para que a instância receba os eventos de passkey (#105).
func TestPasskeyIsPartOfAllEventTypes(t *testing.T) {
	if !event_types.IsEventType(event_types.PASSKEY) {
		t.Fatal("PASSKEY deve ser um tipo de evento válido")
	}
	found := false
	for _, e := range event_types.AllEventTypes {
		if e == event_types.PASSKEY {
			found = true
		}
	}
	if !found {
		t.Fatal("PASSKEY deve fazer parte de AllEventTypes")
	}
}

type recordingProducer struct {
	mu    sync.Mutex
	calls []string
}

func (r *recordingProducer) Produce(queueName string, _ []byte, _ string, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, queueName)
	return nil
}
func (r *recordingProducer) CreateGlobalQueues() error { return nil }
func (r *recordingProducer) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func TestCallWebhookDeliversPasskeyEventsOnlyToPasskeySubscribers(t *testing.T) {
	newService := func(producer *recordingProducer) *whatsmeowService {
		return &whatsmeowService{
			config:          &config.Config{},
			loggerWrapper:   logger_wrapper.NewLoggerManager(&config.Config{LogDirectory: t.TempDir()}),
			webhookProducer: producer,
		}
	}
	payload := []byte(`{"event":"PasskeyRequest","data":{"stage":"challenge"}}`)

	t.Run("sem PASSKEY não recebe", func(t *testing.T) {
		p := &recordingProducer{}
		newService(p).CallWebhook(&instance_model.Instance{Id: "i1", Webhook: "http://hook", Events: "CONNECTION,MESSAGE"}, "i1.passkeyrequest", payload)
		if p.count() != 0 {
			t.Fatal("a instância sem PASSKEY recebeu o evento")
		}
	})

	t.Run("com PASSKEY recebe", func(t *testing.T) {
		p := &recordingProducer{}
		newService(p).CallWebhook(&instance_model.Instance{Id: "i1", Webhook: "http://hook", Events: "PASSKEY"}, "i1.passkeyrequest", payload)
		if p.count() != 1 {
			t.Fatalf("o evento chegou %d vezes, esperava 1", p.count())
		}
	})

	t.Run("ALL expandido no connect recebe", func(t *testing.T) {
		p := &recordingProducer{}
		all := ""
		for i, e := range event_types.AllEventTypes {
			if i > 0 {
				all += ","
			}
			all += e
		}
		newService(p).CallWebhook(&instance_model.Instance{Id: "i1", Webhook: "http://hook", Events: all}, "i1.passkeyrequest", payload)
		if p.count() != 1 {
			t.Fatalf("o evento chegou %d vezes com a lista de ALL expandida, esperava 1", p.count())
		}
	})
}
