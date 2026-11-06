package group_service

import (
	"context"
	"errors"
	"fmt"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"github.com/evolution-foundation/evolution-go/pkg/utils"
	"go.mau.fi/whatsmeow/types"
)

// ErrInvalidGroupInvite diz que os dados do convite vieram incompletos.
var ErrInvalidGroupInvite = errors.New("invalid group invite")

// GroupLinkInfoStruct pede os dados de um grupo a partir do link, sem entrar nele.
type GroupLinkInfoStruct struct {
	Code string `json:"code"` // o link inteiro ou só o código
}

// GroupInviteStruct são os campos do cartão de convite (groupInviteMessage)
// recebido em uma conversa, que é diferente do link público.
type GroupInviteStruct struct {
	GroupJID   string `json:"groupJid"`
	Inviter    string `json:"inviter"`
	Code       string `json:"code"`
	Expiration int64  `json:"expiration"`
}

func (d *GroupInviteStruct) parse() (types.JID, types.JID, error) {
	group, ok := utils.ParseJID(d.GroupJID)
	if !ok || group.IsEmpty() || group.Server != types.GroupServer {
		return types.EmptyJID, types.EmptyJID, fmt.Errorf("%w: invalid groupJid", ErrInvalidGroupInvite)
	}
	inviter, ok := utils.ParseJID(d.Inviter)
	if !ok || inviter.IsEmpty() {
		return types.EmptyJID, types.EmptyJID, fmt.Errorf("%w: invalid inviter", ErrInvalidGroupInvite)
	}
	if d.Code == "" {
		return types.EmptyJID, types.EmptyJID, fmt.Errorf("%w: code is required", ErrInvalidGroupInvite)
	}
	return group, utils.CanonicalJID(inviter), nil
}

// GetGroupInfoFromLink devolve os dados do grupo do link sem entrar nele.
func (g *groupService) GetGroupInfoFromLink(data *GroupLinkInfoStruct, instance *instance_model.Instance) (*types.GroupInfo, error) {
	if data.Code == "" {
		return nil, fmt.Errorf("%w: code is required", ErrInvalidGroupInvite)
	}
	client, err := g.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, err
	}
	return client.GetGroupInfoFromLink(context.Background(), data.Code)
}

// GetGroupInfoFromInvite devolve os dados do grupo do cartão sem entrar nele.
func (g *groupService) GetGroupInfoFromInvite(data *GroupInviteStruct, instance *instance_model.Instance) (*types.GroupInfo, error) {
	group, inviter, err := data.parse()
	if err != nil {
		return nil, err
	}
	client, err := g.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, err
	}
	return client.GetGroupInfoFromInvite(context.Background(), group, inviter, data.Code, data.Expiration)
}

// JoinGroupWithInvite aceita o cartão de convite recebido em uma conversa.
func (g *groupService) JoinGroupWithInvite(data *GroupInviteStruct, instance *instance_model.Instance) error {
	group, inviter, err := data.parse()
	if err != nil {
		return err
	}
	client, err := g.ensureClientConnected(instance.Id)
	if err != nil {
		return err
	}
	return client.JoinGroupWithInvite(context.Background(), group, inviter, data.Code, data.Expiration)
}

// parseRequestParticipants lê as pessoas de um pedido de entrada. O pedido vai como
// nó bruto, sem resolução de aparelhos: o "+" de um número solto chegaria ao servidor.
func parseRequestParticipants(participants []string) ([]types.JID, error) {
	result := make([]types.JID, 0, len(participants))
	for _, participant := range participants {
		jid, ok := utils.ParseJID(participant)
		if !ok || jid.IsEmpty() {
			return nil, errors.New("invalid participant jid: " + participant)
		}
		result = append(result, utils.CanonicalJID(jid))
	}
	return result, nil
}
