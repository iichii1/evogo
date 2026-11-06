package send_handler

import (
	"net/http"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	send_service "github.com/evolution-foundation/evolution-go/pkg/sendMessage/service"
	"github.com/gin-gonic/gin"
)

// runSend lê o corpo, confere o destino e envia.
func runSend[T any](ctx *gin.Context, number func(*T) string, send func(*T, *instance_model.Instance) (*send_service.MessageSendStruct, error)) {
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
	if number(&data) == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	message, err := send(&data, instance)
	if err != nil {
		respondSendError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}

// Send several contacts in one card
// @Summary Send several contacts in one card
// @Description Sends up to 50 contacts in a single card. `displayName` defaults to "<n> contatos".
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body send_service.ContactsStruct true "Contacts"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 429 {object} gin.H "Send queue full or wait too long (see the Retry-After header)"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /send/contacts [post]
func (s *sendHandler) SendContacts(ctx *gin.Context) {
	runSend(ctx, func(data *send_service.ContactsStruct) string { return data.Number }, s.sendMessageService.SendContacts)
}

// Send a group invite card
// @Summary Send a group invite card
// @Description Sends the invite card of a group. `code` and `inviteExpiration` come from the AddRequest field of the /group/participant response, when the privacy of the person prevents adding them directly.
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body send_service.GroupInviteCardStruct true "Group invite"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 429 {object} gin.H "Send queue full or wait too long (see the Retry-After header)"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /send/groupInvite [post]
func (s *sendHandler) SendGroupInvite(ctx *gin.Context) {
	runSend(ctx, func(data *send_service.GroupInviteCardStruct) string { return data.Number }, s.sendMessageService.SendGroupInvite)
}

// Ask a contact to share their phone number
// @Summary Ask a contact to share their phone number
// @Description Shows the contact a request to share their phone number. Useful for contacts known only by LID.
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body send_service.RequestPhoneStruct true "Contact"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 429 {object} gin.H "Send queue full or wait too long (see the Retry-After header)"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /send/requestPhone [post]
func (s *sendHandler) SendRequestPhone(ctx *gin.Context) {
	runSend(ctx, func(data *send_service.RequestPhoneStruct) string { return data.Number }, s.sendMessageService.SendRequestPhone)
}
