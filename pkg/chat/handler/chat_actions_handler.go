package chat_handler

import (
	"errors"
	"net/http"

	chat_service "github.com/evolution-foundation/evolution-go/pkg/chat/service"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"github.com/gin-gonic/gin"
)

// runChatAction lê o corpo, executa a ação e responde: 400 para o que o serviço
// recusa antes de falar com o WhatsApp, 500 para o resto.
func runChatAction[T any](ctx *gin.Context, action func(*T, *instance_model.Instance) error) {
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

	if err := action(&data, instance); err != nil {
		if errors.Is(err, chat_service.ErrInvalidChatAction) {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success"})
}

// Mark a chat as read
// @Summary Mark a chat as read
// @Description Marks the whole chat as read on every device of the account. To mark specific messages, use /message/markread.
// @Tags Chat
// @Accept json
// @Produce json
// @Param message body chat_service.ChatRangeStruct true "Chat"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /chat/markread [post]
func (c *chatHandler) ChatMarkRead(ctx *gin.Context) {
	runChatAction(ctx, c.chatService.ChatMarkRead)
}

// Mark a chat as unread
// @Summary Mark a chat as unread
// @Description Marks the whole chat as unread on every device of the account.
// @Tags Chat
// @Accept json
// @Produce json
// @Param message body chat_service.ChatRangeStruct true "Chat"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /chat/markunread [post]
func (c *chatHandler) ChatMarkUnread(ctx *gin.Context) {
	runChatAction(ctx, c.chatService.ChatMarkUnread)
}

// Delete a chat
// @Summary Delete a chat
// @Description Deletes the chat from every device of the account. `deleteMedia` also removes its media from the phone.
// @Tags Chat
// @Accept json
// @Produce json
// @Param message body chat_service.ChatRangeStruct true "Chat"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /chat/delete [post]
func (c *chatHandler) ChatDelete(ctx *gin.Context) {
	runChatAction(ctx, c.chatService.ChatDelete)
}

// Star a message
// @Summary Star a message
// @Description Stars a message on every device of the account.
// @Tags Chat
// @Accept json
// @Produce json
// @Param message body chat_service.StarStruct true "Message"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /chat/star [post]
func (c *chatHandler) MessageStar(ctx *gin.Context) {
	runChatAction(ctx, c.chatService.MessageStar)
}

// Unstar a message
// @Summary Unstar a message
// @Description Removes the star of a message on every device of the account.
// @Tags Chat
// @Accept json
// @Produce json
// @Param message body chat_service.StarStruct true "Message"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /chat/unstar [post]
func (c *chatHandler) MessageUnstar(ctx *gin.Context) {
	runChatAction(ctx, c.chatService.MessageUnstar)
}

// Set disappearing messages of a chat
// @Summary Set disappearing messages of a chat
// @Description Sets the disappearing messages timer of a direct chat or group. `timer` is off, 24h, 7d or 90d.
// @Tags Chat
// @Accept json
// @Produce json
// @Param message body chat_service.DisappearingStruct true "Chat and timer"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /chat/disappearing [post]
func (c *chatHandler) SetDisappearing(ctx *gin.Context) {
	runChatAction(ctx, c.chatService.SetDisappearing)
}
