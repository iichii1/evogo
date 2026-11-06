package send_service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"github.com/evolution-foundation/evolution-go/pkg/utils"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// ErrInvalidSendInput diz que o corpo da requisição não pode virar uma mensagem:
// o handler responde 400, em vez dos 500 dos erros que vêm do WhatsApp.
var ErrInvalidSendInput = errors.New("invalid message")

// maxContactsInCard é uma trava nossa contra um cartão gigante, não um limite
// documentado pelo WhatsApp.
const maxContactsInCard = 50

// ContactsStruct envia vários contatos num só cartão.
type ContactsStruct struct {
	Number      string              `json:"number"`
	Id          string              `json:"id"`
	DisplayName string              `json:"displayName,omitempty"` // padrão: "<n> contatos"
	Contacts    []utils.VCardStruct `json:"contacts"`
	Delay       int32               `json:"delay"`
	FormatJid   *bool               `json:"formatJid,omitempty"`
	Quoted      QuotedStruct        `json:"quoted"`
	Expiration  *uint32             `json:"expiration,omitempty"`
}

// GroupInviteCardStruct envia o cartão de convite para um grupo. Code e
// InviteExpiration vêm do campo AddRequest da resposta de /group/participant,
// quando a privacidade da pessoa impede adicioná-la direto.
type GroupInviteCardStruct struct {
	Number           string       `json:"number"`
	Id               string       `json:"id"`
	GroupJID         string       `json:"groupJid"`
	Code             string       `json:"code"`
	InviteExpiration int64        `json:"inviteExpiration"`
	GroupName        string       `json:"groupName"`
	Caption          string       `json:"caption,omitempty"`
	Delay            int32        `json:"delay"`
	FormatJid        *bool        `json:"formatJid,omitempty"`
	Quoted           QuotedStruct `json:"quoted"`
	Expiration       *uint32      `json:"expiration,omitempty"`
}

// RequestPhoneStruct pede ao contato que compartilhe o telefone.
type RequestPhoneStruct struct {
	Number     string       `json:"number"`
	Id         string       `json:"id"`
	Delay      int32        `json:"delay"`
	FormatJid  *bool        `json:"formatJid,omitempty"`
	Quoted     QuotedStruct `json:"quoted"`
	Expiration *uint32      `json:"expiration,omitempty"`
}

func invalidSend(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrInvalidSendInput}, args...)...)
}

// buildContactsMessage monta o cartão. Uma quebra de linha num campo acrescentaria
// linhas ao vCard, então é recusada.
func buildContactsMessage(data *ContactsStruct) (*waE2E.Message, error) {
	if len(data.Contacts) == 0 {
		return nil, invalidSend("contacts is required")
	}
	if len(data.Contacts) > maxContactsInCard {
		return nil, invalidSend("at most %d contacts in a card", maxContactsInCard)
	}

	contacts := make([]*waE2E.ContactMessage, 0, len(data.Contacts))
	for i, contact := range data.Contacts {
		if contact.FullName == "" || contact.Phone == "" {
			return nil, invalidSend("contact %d needs fullName and phone", i)
		}
		if strings.ContainsAny(contact.FullName+contact.Phone+contact.Organization, "\r\n") {
			return nil, invalidSend("contact %d has a line break", i)
		}
		vcard := utils.GenerateVC(contact)
		contacts = append(contacts, &waE2E.ContactMessage{
			DisplayName: proto.String(contact.FullName),
			Vcard:       proto.String(vcard),
		})
	}

	displayName := data.DisplayName
	if displayName == "" {
		displayName = fmt.Sprintf("%d contatos", len(contacts))
	}
	return &waE2E.Message{ContactsArrayMessage: &waE2E.ContactsArrayMessage{
		DisplayName: proto.String(displayName),
		Contacts:    contacts,
	}}, nil
}

func buildGroupInviteMessage(data *GroupInviteCardStruct) (*waE2E.Message, error) {
	group, ok := utils.ParseJID(data.GroupJID)
	if !ok || group.Server != types.GroupServer {
		return nil, invalidSend("invalid groupJid")
	}
	if data.Code == "" {
		return nil, invalidSend("code is required")
	}
	if data.GroupName == "" {
		return nil, invalidSend("groupName is required")
	}

	invite := &waE2E.GroupInviteMessage{
		GroupJID:   proto.String(group.String()),
		InviteCode: proto.String(data.Code),
		GroupName:  proto.String(data.GroupName),
	}
	if data.InviteExpiration > 0 {
		invite.InviteExpiration = proto.Int64(data.InviteExpiration)
	}
	if data.Caption != "" {
		invite.Caption = proto.String(data.Caption)
	}
	return &waE2E.Message{GroupInviteMessage: invite}, nil
}

// parseStatusColor lê #RRGGBB ou #AARRGGBB; sem o canal alfa, a cor fica opaca.
func parseStatusColor(value string) (uint32, error) {
	hex := strings.TrimPrefix(value, "#")
	if len(hex) != 6 && len(hex) != 8 {
		return 0, invalidSend("color %q must be #RRGGBB or #AARRGGBB", value)
	}
	parsed, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return 0, invalidSend("color %q must be #RRGGBB or #AARRGGBB", value)
	}
	color := uint32(parsed)
	if len(hex) == 6 {
		color |= 0xFF000000
	}
	return color, nil
}

// buildStatusTextMessage monta o Status de texto. Cor e fonte são opcionais.
func buildStatusTextMessage(data *StatusTextStruct) (*waE2E.Message, error) {
	text := &waE2E.ExtendedTextMessage{Text: proto.String(data.Text)}
	if data.BackgroundColor != "" {
		color, err := parseStatusColor(data.BackgroundColor)
		if err != nil {
			return nil, err
		}
		text.BackgroundArgb = proto.Uint32(color)
	}
	if data.TextColor != "" {
		color, err := parseStatusColor(data.TextColor)
		if err != nil {
			return nil, err
		}
		text.TextArgb = proto.Uint32(color)
	}
	if data.Font != nil {
		if _, ok := waE2E.ExtendedTextMessage_FontType_name[*data.Font]; !ok {
			return nil, invalidSend("unknown font %d", *data.Font)
		}
		text.Font = waE2E.ExtendedTextMessage_FontType(*data.Font).Enum()
	}
	return &waE2E.Message{ExtendedTextMessage: text}, nil
}

// SendContacts envia vários contatos num só cartão.
func (s *sendService) SendContacts(data *ContactsStruct, instance *instance_model.Instance) (*MessageSendStruct, error) {
	msg, err := buildContactsMessage(data)
	if err != nil {
		return nil, err
	}
	if _, err := s.ensureClientConnected(instance.Id); err != nil {
		return nil, err
	}
	return s.SendMessage(instance, msg, "ContactsArrayMessage", &SendDataStruct{
		Id:         data.Id,
		Number:     data.Number,
		Quoted:     data.Quoted,
		Delay:      data.Delay,
		FormatJid:  data.FormatJid,
		Expiration: data.Expiration,
	})
}

// SendGroupInvite envia o cartão de convite para um grupo.
func (s *sendService) SendGroupInvite(data *GroupInviteCardStruct, instance *instance_model.Instance) (*MessageSendStruct, error) {
	msg, err := buildGroupInviteMessage(data)
	if err != nil {
		return nil, err
	}
	if _, err := s.ensureClientConnected(instance.Id); err != nil {
		return nil, err
	}
	return s.SendMessage(instance, msg, "GroupInviteMessage", &SendDataStruct{
		Id:         data.Id,
		Number:     data.Number,
		Quoted:     data.Quoted,
		Delay:      data.Delay,
		FormatJid:  data.FormatJid,
		Expiration: data.Expiration,
	})
}

// SendRequestPhone pede ao contato que compartilhe o telefone.
func (s *sendService) SendRequestPhone(data *RequestPhoneStruct, instance *instance_model.Instance) (*MessageSendStruct, error) {
	if _, err := s.ensureClientConnected(instance.Id); err != nil {
		return nil, err
	}
	msg := &waE2E.Message{RequestPhoneNumberMessage: &waE2E.RequestPhoneNumberMessage{}}
	return s.SendMessage(instance, msg, "RequestPhoneNumberMessage", &SendDataStruct{
		Id:         data.Id,
		Number:     data.Number,
		Quoted:     data.Quoted,
		Delay:      data.Delay,
		FormatJid:  data.FormatJid,
		Expiration: data.Expiration,
	})
}
