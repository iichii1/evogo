package routes

import "github.com/gin-gonic/gin"

// As rotas /license/* são registradas dentro de pkg/core (LicenseRoutes) como
// funções anônimas, onde não há como anotá-las para o swag. As funções abaixo
// existem só para que a documentação Swagger delas continue sendo gerada; elas
// não são registradas em nenhuma rota e nunca são chamadas.

var _ = []gin.HandlerFunc{licenseStatusDoc, licenseRegisterDoc, licenseActivateDoc}

// licenseStatusDoc documenta GET /license/status.
// @Summary Get license status
// @Description Returns whether the instance license is active, along with the instance id and a masked api key.
// @Tags License
// @Produce json
// @Success 200 {object} gin.H "License status ({status, instance_id, api_key?})"
// @Router /license/status [get]
func licenseStatusDoc(*gin.Context) {}

// licenseRegisterDoc documenta GET /license/register.
// @Summary Register / get registration URL
// @Description Checks the GLOBAL_API_KEY with the licensing server. If not yet registered, initiates registration and returns a register_url. Accepts an optional redirect_uri for the post-registration redirect.
// @Tags License
// @Produce json
// @Param redirect_uri query string false "Post-registration redirect URI"
// @Success 200 {object} gin.H "Registration state (status/message or register_url)"
// @Router /license/register [get]
func licenseRegisterDoc(*gin.Context) {}

// licenseActivateDoc documenta GET /license/activate.
// @Summary Activate license
// @Description Exchanges an authorization code (from the registration callback) for an api_key and persists it. Provide the code via the query string.
// @Tags License
// @Produce json
// @Param code query string true "Authorization code from the registration callback"
// @Success 200 {object} gin.H "Activation result"
// @Failure 400 {object} gin.H "Missing code parameter"
// @Router /license/activate [get]
func licenseActivateDoc(*gin.Context) {}
