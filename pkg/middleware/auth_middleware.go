package auth_middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	instance_service "github.com/evolution-foundation/evolution-go/pkg/instance/service"
	"github.com/gin-gonic/gin"
)

// adminContextKey marca, no contexto, que a requisição veio com a chave global.
const adminContextKey = "admin"

type Middleware interface {
	Auth(ctx *gin.Context)
	AuthAdmin(ctx *gin.Context)
	// AuthInstanceOrAdmin aceita o apikey de uma instância (que só pode agir sobre
	// ela mesma) ou a chave global (que pode agir sobre qualquer instância).
	AuthInstanceOrAdmin(ctx *gin.Context)
}

// IsAdmin informa se a requisição foi autenticada com a chave global.
func IsAdmin(ctx *gin.Context) bool {
	return ctx.GetBool(adminContextKey)
}

// MatchesGlobalKey informa se supplied é a chave global configurada.
//
// A comparação leva o mesmo tempo seja qual for o ponto em que as chaves
// diferem, para que a chave não possa ser descoberta medindo o tempo de
// resposta. Sem chave global configurada nada coincide, nem uma chave vazia.
func MatchesGlobalKey(configured, supplied string) bool {
	if configured == "" || supplied == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(supplied), []byte(configured)) == 1
}

type middleware struct {
	config          *config.Config
	instanceService instance_service.InstanceService
}

func (m middleware) Auth(ctx *gin.Context) {
	token := ctx.GetHeader("apikey")
	if token == "" {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
		return
	}

	instance, err := m.instanceService.GetInstanceByToken(token)
	if err != nil {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
		return
	}

	ctx.Set("instance", instance)

	ctx.Next()
}

func (m middleware) AuthAdmin(ctx *gin.Context) {
	token := ctx.GetHeader("apikey")
	if token == "" {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
		return
	}

	if !MatchesGlobalKey(m.config.GlobalApiKey, token) {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
		return
	}

	ctx.Next()
}

func (m middleware) AuthInstanceOrAdmin(ctx *gin.Context) {
	token := ctx.GetHeader("apikey")
	if token == "" {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
		return
	}

	if MatchesGlobalKey(m.config.GlobalApiKey, token) {
		ctx.Set(adminContextKey, true)
		ctx.Next()
		return
	}

	instance, err := m.instanceService.GetInstanceByToken(token)
	if err != nil {
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
		return
	}

	ctx.Set("instance", instance)
	ctx.Next()
}

func NewMiddleware(config *config.Config, instanceService instance_service.InstanceService) *middleware {
	return &middleware{config: config, instanceService: instanceService}
}
