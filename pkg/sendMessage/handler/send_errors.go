package send_handler

import (
	"errors"
	"math"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	send_service "github.com/evolution-foundation/evolution-go/pkg/sendMessage/service"
	"github.com/evolution-foundation/evolution-go/pkg/sendqueue"
)

// respondSendError responde a um erro de envio. A fila de envio recusada vira
// 429 com o cabeçalho Retry-After (em segundos), para quem envia em lote saber
// quando tentar de novo; todo o resto segue como antes, 500.
func respondSendError(ctx *gin.Context, err error) {
	if errors.Is(err, send_service.ErrInvalidSendInput) {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var rejected *sendqueue.RejectedError
	if errors.As(err, &rejected) {
		seconds := int(math.Ceil(rejected.RetryAfter.Seconds()))
		if seconds < 1 {
			seconds = 1
		}
		ctx.Header("Retry-After", strconv.Itoa(seconds))
		ctx.JSON(http.StatusTooManyRequests, gin.H{
			"error":      err.Error(),
			"retryAfter": seconds,
		})
		return
	}
	ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}
