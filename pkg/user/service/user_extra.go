package user_service

import (
	"context"
	"errors"
	"fmt"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"github.com/evolution-foundation/evolution-go/pkg/utils"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// ErrInvalidUserRequest diz que o corpo da requisição veio incompleto ou inválido.
var ErrInvalidUserRequest = errors.New("invalid request")

// BusinessProfileStruct identifica o contato cujo perfil comercial será lido.
type BusinessProfileStruct struct {
	Number string `json:"number"`
}

// DefaultDisappearingStruct define o tempo padrão das conversas novas da conta.
type DefaultDisappearingStruct struct {
	Timer string `json:"timer"` // off, 24h, 7d ou 90d
}

func parseBusinessJID(number string) (types.JID, error) {
	jid, ok := utils.ParseJID(number)
	if !ok || jid.IsEmpty() {
		return types.EmptyJID, fmt.Errorf("%w: invalid number", ErrInvalidUserRequest)
	}
	return utils.CanonicalJID(jid), nil
}

// GetBusinessProfile lê endereço, e-mail, categorias e horário de um perfil comercial.
func (u *userService) GetBusinessProfile(data *BusinessProfileStruct, instance *instance_model.Instance) (*types.BusinessProfile, error) {
	jid, err := parseBusinessJID(data.Number)
	if err != nil {
		return nil, err
	}
	client, err := u.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, err
	}
	return client.GetBusinessProfile(context.Background(), jid)
}

// SetDefaultDisappearing define o tempo das mensagens temporárias das conversas novas.
func (u *userService) SetDefaultDisappearing(data *DefaultDisappearingStruct, instance *instance_model.Instance) error {
	timer, ok := whatsmeow.ParseDisappearingTimerString(data.Timer)
	if !ok {
		return fmt.Errorf("%w: timer must be off, 24h, 7d or 90d", ErrInvalidUserRequest)
	}
	client, err := u.ensureClientConnected(instance.Id)
	if err != nil {
		return err
	}
	return client.SetDefaultDisappearingTimer(context.Background(), timer)
}
