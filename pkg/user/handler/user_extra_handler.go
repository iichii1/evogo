package user_handler

import (
	"errors"
	"net/http"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	user_service "github.com/evolution-foundation/evolution-go/pkg/user/service"
	"github.com/gin-gonic/gin"
)

func userError(ctx *gin.Context, err error) {
	if errors.Is(err, user_service.ErrInvalidUserRequest) {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

// Get a business profile
// @Summary Get a business profile
// @Description Returns the address, e-mail, categories and business hours of a WhatsApp Business contact.
// @Tags User
// @Accept json
// @Produce json
// @Param message body user_service.BusinessProfileStruct true "Contact"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /user/business [post]
func (u *userHandler) GetBusinessProfile(ctx *gin.Context) {
	instance, ok := ctx.MustGet("instance").(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data user_service.BusinessProfileStruct
	if err := ctx.ShouldBindBodyWithJSON(&data); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	profile, err := u.userService.GetBusinessProfile(&data, instance)
	if err != nil {
		userError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": profile})
}

// Set the default disappearing messages timer
// @Summary Set the default disappearing messages timer
// @Description Sets the timer applied to new chats of the account. `timer` is off, 24h, 7d or 90d.
// @Tags User
// @Accept json
// @Produce json
// @Param message body user_service.DefaultDisappearingStruct true "Timer"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /user/disappearing [post]
func (u *userHandler) SetDefaultDisappearing(ctx *gin.Context) {
	instance, ok := ctx.MustGet("instance").(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data user_service.DefaultDisappearingStruct
	if err := ctx.ShouldBindBodyWithJSON(&data); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := u.userService.SetDefaultDisappearing(&data, instance); err != nil {
		userError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success"})
}
