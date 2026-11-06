package chat_service

import (
	"context"
	"errors"
	"fmt"
	"time"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"github.com/evolution-foundation/evolution-go/pkg/utils"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// ErrInvalidChatAction diz que o corpo da requisição não pode virar uma ação: o
// handler responde 400, em vez dos 500 dos erros que vêm do WhatsApp.
var ErrInvalidChatAction = errors.New("invalid chat action")

// ChatRangeStruct identifica a conversa e, de forma opcional, a última mensagem
// dela. O WhatsApp usa essa mensagem para saber até onde a ação vale nos outros
// aparelhos; sem ela, vale até o horário informado ou o de agora.
type ChatRangeStruct struct {
	Chat                 string `json:"chat"`
	LastMessageId        string `json:"lastMessageId,omitempty"`
	LastMessageTimestamp int64  `json:"lastMessageTimestamp,omitempty"` // segundos desde 1970
	LastMessageFromMe    bool   `json:"lastMessageFromMe,omitempty"`
	LastMessageSender    string `json:"lastMessageSender,omitempty"` // obrigatório em grupos, se a mensagem não for sua
	DeleteMedia          bool   `json:"deleteMedia,omitempty"`       // só em /chat/delete
}

// StarStruct identifica a mensagem a favoritar ou desfavoritar.
type StarStruct struct {
	Chat   string `json:"chat"`
	Id     string `json:"id"`
	Sender string `json:"sender,omitempty"` // autor da mensagem, em grupos
	FromMe bool   `json:"fromMe"`
}

// DisappearingStruct define o tempo das mensagens temporárias de uma conversa.
type DisappearingStruct struct {
	Chat  string `json:"chat"`
	Timer string `json:"timer"` // off, 24h, 7d ou 90d
}

// canonicalJID lê um número ou JID e tira o "+" que utils.ParseJID acrescenta a um
// número solto. O app state usa o JID como está no índice da alteração: com o "+",
// a ação não encontra a conversa nos outros aparelhos.
func canonicalJID(value string) (types.JID, bool) {
	jid, ok := utils.ParseJID(value)
	if !ok || jid.IsEmpty() {
		return types.EmptyJID, false
	}
	return utils.CanonicalJID(jid), true
}

func parseChat(chat string) (types.JID, error) {
	jid, ok := canonicalJID(chat)
	if !ok {
		return types.EmptyJID, fmt.Errorf("%w: invalid chat", ErrInvalidChatAction)
	}
	return jid, nil
}

// lastMessage devolve o horário e a chave da última mensagem da conversa.
func (r *ChatRangeStruct) lastMessage(chat types.JID) (time.Time, *waCommon.MessageKey, error) {
	var timestamp time.Time
	if r.LastMessageTimestamp > 0 {
		timestamp = time.Unix(r.LastMessageTimestamp, 0)
	}
	if r.LastMessageId == "" {
		return timestamp, nil, nil
	}

	key := &waCommon.MessageKey{
		RemoteJID: proto.String(chat.String()),
		FromMe:    proto.Bool(r.LastMessageFromMe),
		ID:        proto.String(r.LastMessageId),
	}
	if chat.Server == types.GroupServer && !r.LastMessageFromMe {
		sender, ok := canonicalJID(r.LastMessageSender)
		if !ok {
			return time.Time{}, nil, fmt.Errorf("%w: lastMessageSender is required in groups", ErrInvalidChatAction)
		}
		key.Participant = proto.String(sender.String())
	}
	return timestamp, key, nil
}

func buildMarkChatPatch(data *ChatRangeStruct, read bool) (appstate.PatchInfo, error) {
	chat, err := parseChat(data.Chat)
	if err != nil {
		return appstate.PatchInfo{}, err
	}
	timestamp, key, err := data.lastMessage(chat)
	if err != nil {
		return appstate.PatchInfo{}, err
	}
	return appstate.BuildMarkChatAsRead(chat, read, timestamp, key), nil
}

func buildDeleteChatPatch(data *ChatRangeStruct) (appstate.PatchInfo, error) {
	chat, err := parseChat(data.Chat)
	if err != nil {
		return appstate.PatchInfo{}, err
	}
	timestamp, key, err := data.lastMessage(chat)
	if err != nil {
		return appstate.PatchInfo{}, err
	}
	return appstate.BuildDeleteChat(chat, timestamp, key, data.DeleteMedia), nil
}

func buildStarPatch(data *StarStruct, starred bool) (appstate.PatchInfo, error) {
	chat, err := parseChat(data.Chat)
	if err != nil {
		return appstate.PatchInfo{}, err
	}
	if data.Id == "" {
		return appstate.PatchInfo{}, fmt.Errorf("%w: id is required", ErrInvalidChatAction)
	}

	// Em conversa individual o autor é a própria conversa: BuildStar grava "0".
	sender := chat
	if chat.Server == types.GroupServer && !data.FromMe {
		parsed, ok := canonicalJID(data.Sender)
		if !ok {
			return appstate.PatchInfo{}, fmt.Errorf("%w: sender is required in groups", ErrInvalidChatAction)
		}
		sender = parsed
	}
	return appstate.BuildStar(chat, sender, data.Id, data.FromMe, starred), nil
}

// ParseDisappearingTimer aceita só os tempos que o WhatsApp mostra: os aplicativos
// oficiais ignoram qualquer outro, e em grupos o servidor recusa.
func ParseDisappearingTimer(value string) (time.Duration, error) {
	timer, ok := whatsmeow.ParseDisappearingTimerString(value)
	if !ok {
		return 0, fmt.Errorf("%w: timer must be off, 24h, 7d or 90d", ErrInvalidChatAction)
	}
	return timer, nil
}

func (c *chatService) sendPatch(instance *instance_model.Instance, patch appstate.PatchInfo, buildErr error, action string) error {
	if buildErr != nil {
		return buildErr
	}
	client, err := c.ensureClientConnected(instance.Id)
	if err != nil {
		return err
	}
	if err := client.SendAppState(context.Background(), patch); err != nil {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] error %s: %v", instance.Id, action, err)
		return err
	}
	return nil
}

func (c *chatService) ChatMarkRead(data *ChatRangeStruct, instance *instance_model.Instance) error {
	patch, err := buildMarkChatPatch(data, true)
	return c.sendPatch(instance, patch, err, "mark chat as read")
}

func (c *chatService) ChatMarkUnread(data *ChatRangeStruct, instance *instance_model.Instance) error {
	patch, err := buildMarkChatPatch(data, false)
	return c.sendPatch(instance, patch, err, "mark chat as unread")
}

func (c *chatService) ChatDelete(data *ChatRangeStruct, instance *instance_model.Instance) error {
	patch, err := buildDeleteChatPatch(data)
	return c.sendPatch(instance, patch, err, "delete chat")
}

func (c *chatService) MessageStar(data *StarStruct, instance *instance_model.Instance) error {
	patch, err := buildStarPatch(data, true)
	return c.sendPatch(instance, patch, err, "star message")
}

func (c *chatService) MessageUnstar(data *StarStruct, instance *instance_model.Instance) error {
	patch, err := buildStarPatch(data, false)
	return c.sendPatch(instance, patch, err, "unstar message")
}

func (c *chatService) SetDisappearing(data *DisappearingStruct, instance *instance_model.Instance) error {
	chat, err := parseChat(data.Chat)
	if err != nil {
		return err
	}
	if chat.Server != types.DefaultUserServer && chat.Server != types.HiddenUserServer && chat.Server != types.GroupServer {
		return fmt.Errorf("%w: only direct chats and groups have disappearing messages", ErrInvalidChatAction)
	}
	timer, err := ParseDisappearingTimer(data.Timer)
	if err != nil {
		return err
	}
	client, err := c.ensureClientConnected(instance.Id)
	if err != nil {
		return err
	}
	if err := client.SetDisappearingTimer(context.Background(), chat, timer, time.Now()); err != nil {
		c.loggerWrapper.GetLogger(instance.Id).LogError("[%s] error setting disappearing timer: %v", instance.Id, err)
		return err
	}
	return nil
}
