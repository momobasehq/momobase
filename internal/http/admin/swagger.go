package admin

// swaggerLogout documents administrator logout.
//
// @Summary Log out an administrator
// @Tags Admin - Authentication
// @Produce json
// @Security BearerAuth
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/logout [post]
func swaggerLogout() {}

// swaggerMe documents the current administrator endpoint.
//
// @Summary Get the current administrator
// @Tags Admin - Authentication
// @Produce json
// @Security BearerAuth
// @Success 200 {object} apidoc.DocResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/me [get]
func swaggerMe() {}

// swaggerListTransactions documents transaction administration.
//
// @Summary List transactions
// @Tags Admin - Transactions
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number" minimum(1)
// @Param per_page query int false "Items per page" minimum(1) maximum(100)
// @Success 200 {object} apidoc.DocResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Failure 500 {object} apidoc.ErrorResponse
// @Router /api/admin/transactions [get]
func swaggerListTransactions() {}

// swaggerListAuditLogs documents audit log administration.
//
// @Summary List audit logs
// @Tags Admin - Audit
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number" minimum(1)
// @Param per_page query int false "Items per page" minimum(1) maximum(100)
// @Success 200 {object} apidoc.DocResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Failure 500 {object} apidoc.ErrorResponse
// @Router /api/admin/audit-logs [get]
func swaggerListAuditLogs() {}

// swaggerListProviderHealth documents provider health administration.
//
// @Summary List provider health snapshots
// @Tags Admin - Providers
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number" minimum(1)
// @Param per_page query int false "Items per page" minimum(1) maximum(100)
// @Success 200 {object} apidoc.DocResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Failure 500 {object} apidoc.ErrorResponse
// @Router /api/admin/health/providers [get]
func swaggerListProviderHealth() {}

// swaggerActiveProviderBalances documents active provider balance queries.
//
// @Summary Query all active provider balances
// @Tags Admin - Providers
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number" minimum(1)
// @Param per_page query int false "Items per page" minimum(1) maximum(100)
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/balances/providers [get]
func swaggerActiveProviderBalances() {}

// swaggerSystemInfo documents service metadata.
//
// @Summary Get system information
// @Tags Admin - System
// @Produce json
// @Security BearerAuth
// @Success 200 {object} apidoc.DocResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/system/info [get]
func swaggerSystemInfo() {}

// swaggerSystemHealth documents system health.
//
// @Summary Get system health
// @Tags Admin - System
// @Produce json
// @Security BearerAuth
// @Success 200 {object} apidoc.DocResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Failure 500 {object} apidoc.ErrorResponse
// @Router /api/admin/system/health [get]
func swaggerSystemHealth() {}

// swaggerWorkers documents configured workers.
//
// @Summary List configured workers
// @Tags Admin - System
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number" minimum(1)
// @Param per_page query int false "Items per page" minimum(1) maximum(100)
// @Success 200 {object} apidoc.DocResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/workers [get]
func swaggerWorkers() {}

// swaggerRuntimeProviders documents initialized provider runtimes.
//
// @Summary List provider runtimes
// @Tags Admin - System
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number" minimum(1)
// @Param per_page query int false "Items per page" minimum(1) maximum(100)
// @Success 200 {object} apidoc.DocResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Failure 500 {object} apidoc.ErrorResponse
// @Router /api/admin/runtime/providers [get]
func swaggerRuntimeProviders() {}

// swaggerListAdminUsers documents administrator listing.
//
// @Summary List administrators
// @Tags Admin - Users
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number" minimum(1)
// @Param per_page query int false "Items per page" minimum(1) maximum(100)
// @Success 200 {object} apidoc.DocResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Failure 500 {object} apidoc.ErrorResponse
// @Router /api/admin/users [get]
func swaggerListAdminUsers() {}

// swaggerCreateAdminUser documents administrator creation.
//
// @Summary Create an administrator
// @Tags Admin - Users
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body apidoc.CreateAdminRequest true "Administrator"
// @Success 201 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/users [post]
func swaggerCreateAdminUser() {}

// swaggerChangeAdminPassword documents administrator password changes.
//
// @Summary Change an administrator password
// @Tags Admin - Users
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Administrator ID"
// @Param request body apidoc.ChangePasswordRequest true "New password"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/users/{id}/password [patch]
func swaggerChangeAdminPassword() {}

// swaggerChangeAdminStatus documents administrator status changes.
//
// @Summary Change an administrator status
// @Tags Admin - Users
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Administrator ID"
// @Param request body apidoc.ChangeStatusRequest true "Status" SchemaExample({"status":"active"})
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/users/{id}/status [patch]
func swaggerChangeAdminStatus() {}

// swaggerListApps documents application listing.
//
// @Summary List applications
// @Tags Admin - Applications
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number" minimum(1)
// @Param per_page query int false "Items per page" minimum(1) maximum(100)
// @Success 200 {object} apidoc.DocResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Failure 500 {object} apidoc.ErrorResponse
// @Router /api/admin/apps [get]
func swaggerListApps() {}

// swaggerCreateApp documents application creation.
//
// @Summary Create an application
// @Tags Admin - Applications
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body apidoc.CreateAppRequest true "Application"
// @Success 201 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/apps [post]
func swaggerCreateApp() {}

// swaggerGetApp documents application lookup.
//
// @Summary Get an application
// @Tags Admin - Applications
// @Produce json
// @Security BearerAuth
// @Param id path string true "Application ID"
// @Success 200 {object} apidoc.DocResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 404 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/apps/{id} [get]
func swaggerGetApp() {}

// swaggerUpdateApp documents application updates.
//
// @Summary Update an application
// @Tags Admin - Applications
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Application ID"
// @Param request body apidoc.UpdateAppRequest true "Application changes"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/apps/{id} [patch]
func swaggerUpdateApp() {}

// swaggerChangeAppStatus documents application status changes.
//
// @Summary Change application status
// @Tags Admin - Applications
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Application ID"
// @Param request body apidoc.ChangeStatusRequest true "Status" SchemaExample({"status":"disabled"})
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/apps/{id}/status [patch]
func swaggerChangeAppStatus() {}

// swaggerListCredentials documents application credential listing.
//
// @Summary List application credentials
// @Tags Admin - Credentials
// @Produce json
// @Security BearerAuth
// @Param id path string true "Application ID"
// @Param page query int false "Page number" minimum(1)
// @Param per_page query int false "Items per page" minimum(1) maximum(100)
// @Success 200 {object} apidoc.DocResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Failure 500 {object} apidoc.ErrorResponse
// @Router /api/admin/apps/{id}/credentials [get]
func swaggerListCredentials() {}

// swaggerCreateCredential documents application credential creation.
//
// @Summary Create an application credential
// @Description Returns the client secret once. Store it securely because it cannot be retrieved later.
// @Tags Admin - Credentials
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Application ID"
// @Param request body apidoc.CreateCredentialRequest true "Credential"
// @Success 201 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/apps/{id}/credentials [post]
func swaggerCreateCredential() {}

// swaggerRevokeCredential documents credential revocation.
//
// @Summary Revoke an application credential
// @Tags Admin - Credentials
// @Produce json
// @Security BearerAuth
// @Param id path string true "Application ID"
// @Param credentialID path string true "Credential ID"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/apps/{id}/credentials/{credentialID}/revoke [patch]
func swaggerRevokeCredential() {}

// swaggerRotateCredential documents credential rotation.
//
// @Summary Rotate an application credential
// @Description Replaces the client secret, revokes existing sessions, and returns the new secret once.
// @Tags Admin - Credentials
// @Produce json
// @Security BearerAuth
// @Param id path string true "Application ID"
// @Param credentialID path string true "Credential ID"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/apps/{id}/credentials/{credentialID}/rotate [post]
func swaggerRotateCredential() {}

// swaggerListProviders documents provider account listing.
//
// @Summary List provider accounts
// @Tags Admin - Providers
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number" minimum(1)
// @Param per_page query int false "Items per page" minimum(1) maximum(100)
// @Success 200 {object} apidoc.DocResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Failure 500 {object} apidoc.ErrorResponse
// @Router /api/admin/providers [get]
func swaggerListProviders() {}

// swaggerCreateProvider documents provider account creation.
//
// @Summary Create a provider account
// @Tags Admin - Providers
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body apidoc.CreateProviderAccountRequest true "Provider account"
// @Success 201 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/providers/accounts [post]
func swaggerCreateProvider() {}

// swaggerUpdateProviderCountries documents provider country updates.
//
// @Summary Update provider countries
// @Tags Admin - Providers
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Provider account ID"
// @Param request body apidoc.UpdateCountriesRequest true "Countries"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/providers/accounts/{id}/countries [patch]
func swaggerUpdateProviderCountries() {}

// swaggerUpdateProviderConfig documents provider configuration updates.
//
// @Summary Update provider configuration
// @Tags Admin - Providers
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Provider account ID"
// @Param request body apidoc.UpdateProviderConfigRequest true "Provider configuration"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/providers/accounts/{id}/config [patch]
func swaggerUpdateProviderConfig() {}

// swaggerActivateProvider documents provider activation.
//
// @Summary Activate a provider account
// @Tags Admin - Providers
// @Produce json
// @Security BearerAuth
// @Param id path string true "Provider account ID"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/providers/accounts/{id}/activate [patch]
func swaggerActivateProvider() {}

// swaggerDeactivateProvider documents provider deactivation.
//
// @Summary Deactivate a provider account
// @Tags Admin - Providers
// @Produce json
// @Security BearerAuth
// @Param id path string true "Provider account ID"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/providers/accounts/{id}/deactivate [patch]
func swaggerDeactivateProvider() {}

// swaggerTestProvider documents provider configuration tests.
//
// @Summary Test provider configuration
// @Tags Admin - Providers
// @Produce json
// @Security BearerAuth
// @Param id path string true "Provider account ID"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/providers/accounts/{id}/test [post]
func swaggerTestProvider() {}

// swaggerProviderBalance documents a provider balance query.
//
// @Summary Query a provider balance
// @Tags Admin - Providers
// @Produce json
// @Security BearerAuth
// @Param id path string true "Provider account ID"
// @Param country query string false "ISO 3166-1 alpha-2 country code"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/providers/accounts/{id}/balance [get]
func swaggerProviderBalance() {}

// swaggerListRoutes documents payment route listing.
//
// @Summary List payment routes
// @Tags Admin - Routes
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number" minimum(1)
// @Param per_page query int false "Items per page" minimum(1) maximum(100)
// @Success 200 {object} apidoc.DocResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Failure 500 {object} apidoc.ErrorResponse
// @Router /api/admin/routes [get]
func swaggerListRoutes() {}

// swaggerCreateRoute documents payment route creation.
//
// @Summary Create a payment route
// @Tags Admin - Routes
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body apidoc.CreateRouteRequest true "Payment route"
// @Success 201 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/routes [post]
func swaggerCreateRoute() {}

// swaggerUpdateRoute documents payment route updates.
//
// @Summary Update a payment route
// @Tags Admin - Routes
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Payment route ID"
// @Param request body apidoc.UpdateRouteRequest true "Route changes"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/routes/{id} [patch]
func swaggerUpdateRoute() {}
