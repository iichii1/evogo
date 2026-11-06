package group_handler

import (
	"errors"
	"net/http"

	group_service "github.com/evolution-foundation/evolution-go/pkg/group/service"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"github.com/gin-gonic/gin"
)

// runGroupAction lê o corpo, executa a ação e responde: 400 para o que o serviço
// recusa antes de falar com o WhatsApp, 500 para o resto.
func runGroupAction[T any](ctx *gin.Context, validate func(*T) string, action func(*T, *instance_model.Instance) (any, error)) {
	instance, ok := ctx.MustGet("instance").(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data T
	if err := ctx.ShouldBindBodyWithJSON(&data); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if validate != nil {
		if problem := validate(&data); problem != "" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": problem})
			return
		}
	}

	result, err := action(&data, instance)
	if err != nil {
		if errors.Is(err, group_service.ErrInvalidGroupInvite) {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if result == nil {
		ctx.JSON(http.StatusOK, gin.H{"message": "success"})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": result})
}

// List requests to join a group
// @Summary List requests to join a group
// @Description Lists who asked to join a group that requires admin approval. The account must be an admin of the group.
// @Tags Group
// @Accept json
// @Produce json
// @Param message body group_service.GetGroupRequestParticipantsStruct true "Group"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /group/requests [post]
func (g *groupHandler) GetGroupRequests(ctx *gin.Context) {
	runGroupAction(ctx,
		func(data *group_service.GetGroupRequestParticipantsStruct) string {
			if data.GroupJID == "" {
				return "groupJid is required"
			}
			return ""
		},
		func(data *group_service.GetGroupRequestParticipantsStruct, instance *instance_model.Instance) (any, error) {
			return g.groupService.GetGroupRequestParticipants(data, instance)
		})
}

// Approve or reject requests to join a group
// @Summary Approve or reject requests to join a group
// @Description Approves or rejects requests to join a group. `action` is approve or reject.
// @Tags Group
// @Accept json
// @Produce json
// @Param message body group_service.UpdateGroupRequestParticipantsStruct true "Group, action and participants"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /group/requests/update [post]
func (g *groupHandler) UpdateGroupRequests(ctx *gin.Context) {
	runGroupAction(ctx,
		func(data *group_service.UpdateGroupRequestParticipantsStruct) string {
			switch {
			case data.GroupJID == "":
				return "groupJid is required"
			case data.Action != "approve" && data.Action != "reject":
				return "action must be approve or reject"
			case len(data.Participants) == 0:
				return "participants is required"
			}
			return ""
		},
		func(data *group_service.UpdateGroupRequestParticipantsStruct, instance *instance_model.Instance) (any, error) {
			return g.groupService.UpdateGroupRequestParticipants(data, instance)
		})
}

// Preview a group from its invite link
// @Summary Preview a group from its invite link
// @Description Returns the group of an invite link without joining it. `code` is the whole link or only its code.
// @Tags Group
// @Accept json
// @Produce json
// @Param message body group_service.GroupLinkInfoStruct true "Invite link"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /group/invite/link [post]
func (g *groupHandler) GetGroupInfoFromLink(ctx *gin.Context) {
	runGroupAction(ctx, nil, func(data *group_service.GroupLinkInfoStruct, instance *instance_model.Instance) (any, error) {
		return g.groupService.GetGroupInfoFromLink(data, instance)
	})
}

// Preview a group from an invite card
// @Summary Preview a group from an invite card
// @Description Returns the group of an invite card received in a chat (groupInviteMessage) without joining it.
// @Tags Group
// @Accept json
// @Produce json
// @Param message body group_service.GroupInviteStruct true "Invite card"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /group/invite/info [post]
func (g *groupHandler) GetGroupInfoFromInvite(ctx *gin.Context) {
	runGroupAction(ctx, nil, func(data *group_service.GroupInviteStruct, instance *instance_model.Instance) (any, error) {
		return g.groupService.GetGroupInfoFromInvite(data, instance)
	})
}

// Accept an invite card
// @Summary Accept an invite card
// @Description Joins the group of an invite card received in a chat (groupInviteMessage).
// @Tags Group
// @Accept json
// @Produce json
// @Param message body group_service.GroupInviteStruct true "Invite card"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /group/invite/join [post]
func (g *groupHandler) JoinGroupWithInvite(ctx *gin.Context) {
	runGroupAction(ctx, nil, func(data *group_service.GroupInviteStruct, instance *instance_model.Instance) (any, error) {
		return nil, g.groupService.JoinGroupWithInvite(data, instance)
	})
}
