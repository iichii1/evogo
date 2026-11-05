package send_service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"github.com/evolution-foundation/evolution-go/pkg/utils"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// PollVoteStruct is the body for POST /send/pollVote.
//
// Votes on an existing poll. The poll must have been sent or received by this
// instance while it was connected, because the vote is encrypted with the
// poll's secret, which WhatsApp only delivers together with the poll.
type PollVoteStruct struct {
	// Chat where the poll is: phone number, group JID (@g.us) or LID (@lid).
	Number string `json:"number" example:"5511999999999"`
	// ID of the poll message (the one returned by /send/poll or received in the webhook).
	PollMessageID string `json:"pollMessageId" example:"3EB0000000000000000008"`
	// Exact text of the options to select. An empty list removes this instance's vote.
	SelectedOptions []string `json:"selectedOptions" example:"Option 1"`
	// true when the poll was created by this instance.
	FromMe bool `json:"fromMe" example:"false"`
	// Who created the poll, for polls in groups created by someone else. Phone number or JID.
	Participant string `json:"participant,omitempty" example:"5511888888888"`
}

// pollVoteTarget resolve o chat e o autor da enquete, que juntos com o ID
// identificam a chave usada para cifrar o voto.
func pollVoteTarget(data *PollVoteStruct, ownID types.JID) (chat types.JID, sender types.JID, err error) {
	if strings.TrimSpace(data.PollMessageID) == "" {
		return chat, sender, errors.New("pollMessageId is required")
	}

	chat, ok := utils.ParseJID(data.Number)
	if !ok {
		return chat, sender, errors.New("invalid phone number")
	}
	chat = utils.CanonicalJID(chat).ToNonAD()

	isGroup := chat.Server == types.GroupServer

	switch {
	case data.FromMe:
		if ownID.IsEmpty() {
			return chat, sender, errors.New("no active session found")
		}
		sender = ownID.ToNonAD()
	case data.Participant != "":
		participant, ok := utils.ParseJID(data.Participant)
		if !ok {
			return chat, sender, errors.New("invalid participant")
		}
		sender = utils.CanonicalJID(participant).ToNonAD()
	case isGroup:
		return chat, sender, errors.New("participant (the poll creator) is required for group polls not created by this instance")
	default:
		// Conversa individual: quem criou a enquete é o próprio contato.
		sender = chat
	}

	return chat, sender, nil
}

// resolvePollSender escolhe entre o LID e o telefone do autor da enquete.
//
// O whatsmeow deriva a chave do voto a partir da identidade de quem vota, e
// usa o LID ou o telefone conforme o servidor do autor. Conversas individuais
// e grupos em modo LID usam LID; grupos antigos em modo telefone usam o
// telefone. A busca da chave da enquete já aceita os dois formatos.
func resolvePollSender(ctx context.Context, client *whatsmeow.Client, chat, sender types.JID) types.JID {
	useLID := true
	if chat.Server == types.GroupServer {
		if info, err := client.GetGroupInfo(ctx, chat); err == nil && info.AddressingMode != types.AddressingModeLID {
			useLID = false
		}
	}

	if !useLID {
		if sender.Server == types.HiddenUserServer {
			if pn, err := client.Store.LIDs.GetPNForLID(ctx, sender); err == nil && !pn.IsEmpty() {
				return pn.ToNonAD()
			}
		}
		return sender
	}

	if sender.Server != types.DefaultUserServer {
		return sender
	}
	if client.Store.ID != nil && sender.User == client.Store.ID.User && !client.Store.LID.IsEmpty() {
		return client.Store.LID.ToNonAD()
	}
	if lid, err := client.Store.LIDs.GetLIDForPN(ctx, sender); err == nil && !lid.IsEmpty() {
		return lid.ToNonAD()
	}
	return sender
}

// SendPollVote vota numa enquete existente (#26).
func (s *sendService) SendPollVote(data *PollVoteStruct, instance *instance_model.Instance) (*MessageSendStruct, error) {
	client, err := s.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, err
	}
	if client.Store.ID == nil {
		return nil, errors.New("no active session found")
	}

	chat, sender, err := pollVoteTarget(data, *client.Store.ID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sender = resolvePollSender(ctx, client, chat, sender)

	pollInfo := &types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat:     chat,
			Sender:   sender,
			IsFromMe: data.FromMe,
			IsGroup:  chat.Server == types.GroupServer,
		},
		ID: data.PollMessageID,
	}

	msg, err := client.BuildPollVote(ctx, pollInfo, data.SelectedOptions)
	if err != nil {
		s.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Failed to build poll vote for %s: %v", instance.Id, data.PollMessageID, err)
		return nil, fmt.Errorf("could not build the vote; the poll must have been sent or received by this instance (check number, fromMe and participant): %w", err)
	}

	messageID := client.GenerateMessageID()
	resp, err := client.SendMessage(ctx, chat, msg, whatsmeow.SendRequestExtra{ID: messageID})
	if err != nil {
		s.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Failed to send poll vote: %v", instance.Id, err)
		return nil, explainSendError(err)
	}

	s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Poll vote sent for %s (%d options)", instance.Id, data.PollMessageID, len(data.SelectedOptions))

	return &MessageSendStruct{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:     chat,
				Sender:   *client.Store.ID,
				IsFromMe: true,
				IsGroup:  chat.Server == types.GroupServer,
			},
			ID:        messageID,
			Timestamp: resp.Timestamp,
		},
		Message: msg,
	}, nil
}
