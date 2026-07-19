package httpx

// swaggerPing documents the liveness endpoint.
//
// @Summary Check liveness
// @Tags System
// @Success 200
// @Router /ping [get]
func swaggerPing() {}

// swaggerHealth documents the lightweight health endpoint.
//
// @Summary Check API health
// @Tags System
// @Produce json
// @Success 200 {object} apidoc.DocResponse
// @Router /healthz [get]
func swaggerHealth() {}

// swaggerClientToken documents application token issuance.
//
// @Summary Issue application tokens
// @Description Validates an active client ID and secret and returns an access/refresh token pair.
// @Tags Authentication
// @Accept x-www-form-urlencoded
// @Produce json
// @Param grant_type formData string true "OAuth grant type" Enums(client_credentials)
// @Param client_id formData string true "Application client ID"
// @Param client_secret formData string true "Application client secret"
// @Success 200 {object} apidoc.TokenResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/v1/token [post]
func swaggerClientToken() {}

// swaggerAppRefreshToken documents application token refresh.
//
// @Summary Refresh application tokens
// @Tags Authentication
// @Accept x-www-form-urlencoded
// @Produce json
// @Param grant_type formData string true "OAuth grant type" Enums(refresh_token)
// @Param refresh_token formData string true "Application refresh token"
// @Success 200 {object} apidoc.TokenResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/v1/token/refresh [post]
func swaggerAppRefreshToken() {}

// swaggerAdminToken documents administrator login.
//
// @Summary Issue administrator tokens
// @Description The grant_type field may be omitted for compatibility with the admin login form.
// @Tags Authentication
// @Accept x-www-form-urlencoded
// @Produce json
// @Param grant_type formData string false "OAuth grant type" Enums(password)
// @Param username formData string true "Administrator email"
// @Param password formData string true "Administrator password"
// @Success 200 {object} apidoc.TokenResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/token [post]
// @Router /api/admin/login [post]
func swaggerAdminToken() {}

// swaggerAdminRefreshToken documents administrator token refresh.
//
// @Summary Refresh administrator tokens
// @Tags Authentication
// @Accept x-www-form-urlencoded
// @Produce json
// @Param grant_type formData string true "OAuth grant type" Enums(refresh_token)
// @Param refresh_token formData string true "Administrator refresh token"
// @Success 200 {object} apidoc.TokenResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/token/refresh [post]
func swaggerAdminRefreshToken() {}
