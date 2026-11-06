package message_service

import (
	"context"
	"errors"
	"fmt"
	"time"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"github.com/evolution-foundation/evolution-go/pkg/utils"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// ErrInvalidPin diz que o corpo da requisição não identifica a mensagem a fixar.
var ErrInvalidPin = errors.New("invalid pin request")

// Os três prazos que o WhatsApp oferece ao fixar uma mensagem.
const (
	pinFor24Hours = uint32(24 * 60 * 60)
	pinFor7Days   = uint32(7 * 24 * 60 * 60)
	pinFor30Days  = uint32(30 * 24 * 60 * 60)
)

// PinStruct identifica a mensagem a fixar ou soltar na conversa, para todos.
type PinStruct struct {
	Number      string `json:"number"`
	Id          string `json:"id"`
	FromMe      bool   `json:"fromMe"`
	Participant string `json:"participant,omitempty"` // autor da mensagem, em grupos
	Duration    uint32 `json:"duration,omitempty"`    // segundos: 86400, 604800 (padrão) ou 2592000
}

// buildPinMessage monta a mensagem que fixa ou solta outra. A chave precisa do JID
// sem o "+" que utils.ParseJID acrescenta, ou não encontra a mensagem na conversa.
func buildPinMessage(data *PinStruct, pin bool, now time.Time) (types.JID, *waE2E.Message, error) {
	parsed, ok := utils.ParseJID(data.Number)
	if !ok || parsed.IsEmpty() {
		return types.EmptyJID, nil, fmt.Errorf("%w: invalid number", ErrInvalidPin)
	}
	chat := utils.CanonicalJID(parsed)
	if data.Id == "" {
		return types.EmptyJID, nil, fmt.Errorf("%w: id is required", ErrInvalidPin)
	}

	key := &waCommon.MessageKey{
		RemoteJID: proto.String(chat.String()),
		FromMe:    proto.Bool(data.FromMe),
		ID:        proto.String(data.Id),
	}
	if chat.Server == types.GroupServer && !data.FromMe {
		author, ok := utils.ParseJID(data.Participant)
		if !ok || author.IsEmpty() {
			return types.EmptyJID, nil, fmt.Errorf("%w: participant is required in groups", ErrInvalidPin)
		}
		key.Participant = proto.String(utils.CanonicalJID(author).String())
	}

	kind := waE2E.PinInChatMessage_UNPIN_FOR_ALL
	duration := uint32(0)
	if pin {
		kind = waE2E.PinInChatMessage_PIN_FOR_ALL
		duration = data.Duration
		switch duration {
		case 0:
			duration = pinFor7Days
		case pinFor24Hours, pinFor7Days, pinFor30Days:
		default:
			return types.EmptyJID, nil, fmt.Errorf("%w: duration must be 86400, 604800 or 2592000", ErrInvalidPin)
		}
	}

	return chat, &waE2E.Message{
		PinInChatMessage: &waE2E.PinInChatMessage{
			Key:               key,
			Type:              kind.Enum(),
			SenderTimestampMS: proto.Int64(now.UnixMilli()),
		},
		MessageContextInfo: &waE2E.MessageContextInfo{MessageAddOnDurationInSecs: proto.Uint32(duration)},
	}, nil
}

func (m *messageService) sendPin(data *PinStruct, pin bool, instance *instance_model.Instance) error {
	chat, msg, err := buildPinMessage(data, pin, time.Now())
	if err != nil {
		return err
	}
	client, err := m.ensureClientConnected(instance.Id)
	if err != nil {
		return err
	}
	_, err = client.SendMessage(context.Background(), chat, msg)
	return err
}

// PinMessage fixa a mensagem na conversa, para todos.
func (m *messageService) PinMessage(data *PinStruct, instance *instance_model.Instance) error {
	return m.sendPin(data, true, instance)
}

// UnpinMessage solta a mensagem fixada na conversa, para todos.
func (m *messageService) UnpinMessage(data *PinStruct, instance *instance_model.Instance) error {
	return m.sendPin(data, false, instance)
}
