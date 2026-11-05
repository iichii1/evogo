package instance_service

import (
	"strings"
	"testing"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"github.com/evolution-foundation/evolution-go/pkg/internal/event_types"
)

func noWarn(string, ...interface{}) {}

func configuredInstance() *instance_model.Instance {
	return &instance_model.Instance{
		Id:              "i1",
		Events:          "MESSAGE,GROUP,CALL",
		Webhook:         "https://hook.example/wa",
		RabbitmqEnable:  "enabled",
		NatsEnable:      "enabled",
		WebSocketEnable: "enabled",
	}
}

// Regressão de #111: connect sem corpo zerava webhook, filas e eventos.
func TestApplyConnectSettingsEmptyBodyKeepsConfiguration(t *testing.T) {
	inst := configuredInstance()

	events := applyConnectSettings(inst, &ConnectStruct{}, noWarn)

	if inst.Events != "MESSAGE,GROUP,CALL" || strings.Join(events, ",") != "MESSAGE,GROUP,CALL" {
		t.Fatalf("events = %q / %v", inst.Events, events)
	}
	if inst.Webhook != "https://hook.example/wa" || inst.RabbitmqEnable != "enabled" ||
		inst.NatsEnable != "enabled" || inst.WebSocketEnable != "enabled" {
		t.Fatalf("configuração foi alterada: %+v", inst)
	}
}

func TestApplyConnectSettingsExplicitValuesWin(t *testing.T) {
	inst := configuredInstance()

	events := applyConnectSettings(inst, &ConnectStruct{
		Subscribe:       []string{"MESSAGE", "bogus", "PASSKEY"},
		WebhookUrl:      "https://novo.example/hook",
		RabbitmqEnable:  "disabled",
		NatsEnable:      "disabled",
		WebSocketEnable: "disabled",
	}, noWarn)

	if inst.Events != "MESSAGE,PASSKEY" || strings.Join(events, ",") != "MESSAGE,PASSKEY" {
		t.Fatalf("events = %q / %v (tipo inválido deve ser descartado)", inst.Events, events)
	}
	if inst.Webhook != "https://novo.example/hook" || inst.RabbitmqEnable != "disabled" ||
		inst.NatsEnable != "disabled" || inst.WebSocketEnable != "disabled" {
		t.Fatalf("valores explícitos não foram aplicados: %+v", inst)
	}
}

func TestApplyConnectSettingsDefaultsToMessageOnFirstConnect(t *testing.T) {
	inst := &instance_model.Instance{Id: "i1"}
	events := applyConnectSettings(inst, &ConnectStruct{}, noWarn)
	if inst.Events != event_types.MESSAGE || len(events) != 1 {
		t.Fatalf("events = %q / %v", inst.Events, events)
	}
}

func TestApplyConnectSettingsAllExpands(t *testing.T) {
	inst := configuredInstance()
	events := applyConnectSettings(inst, &ConnectStruct{Subscribe: []string{"ALL"}}, noWarn)
	if len(events) != len(event_types.AllEventTypes) {
		t.Fatalf("ALL expandiu para %d eventos, esperava %d", len(events), len(event_types.AllEventTypes))
	}
}

func TestApplyConnectSettingsWarnsOnDiscardedType(t *testing.T) {
	inst := configuredInstance()
	var warned []string
	applyConnectSettings(inst, &ConnectStruct{Subscribe: []string{"nope"}}, func(format string, args ...interface{}) {
		warned = append(warned, format)
	})
	if len(warned) != 1 {
		t.Fatalf("avisos = %v", warned)
	}
}
