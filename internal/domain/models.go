package domain

import "time"

// BaseModel contains the identifier and timestamps shared by persistent models.
type BaseModel struct {
	ID        string    `gorm:"primaryKey;size:40" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AdminUser represents an administrator who can access the management API.
type AdminUser struct {
	BaseModel
	Name                string     `gorm:"size:255;not null" json:"name"`
	Email               string     `gorm:"size:255;uniqueIndex;not null" json:"email"`
	PasswordHash        string     `gorm:"type:text" json:"-"`
	Role                string     `gorm:"size:64;not null;default:super_admin" json:"role"`
	Status              string     `gorm:"size:32;not null;default:active" json:"status"`
	PasswordChangedAt   *time.Time `json:"password_changed_at"`
	LastLoginAt         *time.Time `json:"last_login_at"`
	FailedLoginAttempts int        `gorm:"not null;default:0" json:"failed_login_attempts"`
	LockedUntil         *time.Time `json:"locked_until"`
	CreatedBy           string     `gorm:"size:40" json:"created_by"`
	// Permissions is the role's effective permission codes, resolved when the request
	// is authenticated rather than stored. It is not a column: keeping it out of the
	// token and out of the row is what makes a role change take effect on the very
	// next request instead of the next refresh.
	Permissions []string `gorm:"-" json:"permissions,omitempty"`
}

// ActorID returns the identifier to attribute an administrative action to, falling
// back to "system" when the change did not originate from a signed-in administrator
// — a nil actor is normal for seeding and background work, not an error.
func (u *AdminUser) ActorID() string {
	if u == nil || u.ID == "" {
		return "system"
	}
	return u.ID
}

// Permission is one entry of the seeded permission catalogue. Rows are upserted from
// domain.Permissions on every boot, so the table follows the code rather than leading
// it, and a permission is never defined by a migration.
type Permission struct {
	BaseModel
	// Code is the permission a route requires, formatted resource:action.
	Code string `gorm:"size:128;uniqueIndex:idx_permission_code_audience;not null" json:"code"`
	// Audience is AudienceAdmin or AudienceApp. It is part of the unique key because a
	// code may exist in both, meaning different things to an admin and to an app.
	Audience string `gorm:"size:16;uniqueIndex:idx_permission_code_audience;index;not null" json:"audience"`
	// Description explains the permission in an operator-facing list.
	Description string `gorm:"type:text" json:"description"`
}

// Role is a named set of administrative permissions. AdminUser.Role stores the name,
// not an identifier, so a role's name is its identity and renaming is refused.
type Role struct {
	BaseModel
	// Name is what AdminUser.Role refers to.
	Name string `gorm:"size:64;uniqueIndex;not null" json:"name"`
	// Description explains the role in an operator-facing list.
	Description string `gorm:"type:text" json:"description"`
	// System marks a seeded role. System roles cannot be edited or deleted, which is
	// what makes re-synchronising their permissions on every boot safe.
	System bool `gorm:"not null;default:false" json:"system"`
	// Permissions are the codes this role grants.
	Permissions []Permission `gorm:"many2many:role_permissions" json:"permissions"`
}

// AdminSession records a revocable administrator access and refresh token pair.
type AdminSession struct {
	BaseModel
	AdminUserID      string     `gorm:"size:40;index;not null" json:"admin_user_id"`
	TokenHash        string     `gorm:"size:128;uniqueIndex;not null" json:"-"` // OAuth token ID hash, not raw bearer token
	RefreshTokenHash string     `gorm:"size:128;index" json:"-"`
	IPAddress        string     `gorm:"size:64" json:"ip_address"`
	UserAgent        string     `gorm:"type:text" json:"user_agent"`
	ExpiresAt        time.Time  `gorm:"index;not null" json:"expires_at"`
	RevokedAt        *time.Time `json:"revoked_at"`
}

// AuditLog records an administrative or system action against an entity.
type AuditLog struct {
	BaseModel
	ActorID      string `gorm:"size:40;index" json:"actor_id"`
	ActorType    string `gorm:"size:64;index" json:"actor_type"`
	Action       string `gorm:"size:128;index;not null" json:"action"`
	EntityType   string `gorm:"size:128;index" json:"entity_type"`
	EntityID     string `gorm:"size:80;index" json:"entity_id"`
	MetadataJSON string `gorm:"type:text" json:"metadata_json"`
	IPAddress    string `gorm:"size:64" json:"ip_address"`
	UserAgent    string `gorm:"type:text" json:"user_agent"`
}

// App represents an internal product/system that integrates with Momobase.
// Apps only create and query transactions through the public API. They do not receive webhooks.
type App struct {
	BaseModel
	Name        string         `gorm:"size:255;not null" json:"name"`
	Description string         `gorm:"type:text" json:"description"`
	Status      string         `gorm:"size:32;index;not null;default:active" json:"status"`
	Environment string         `gorm:"size:32;index;not null;default:production" json:"environment"`
	Currency    string         `gorm:"size:3;index;not null;default:UGX" json:"currency"`
	Charges     ChargeSchedule `gorm:"embedded" json:"charges"`
	CreatedBy   string         `gorm:"size:40;index" json:"created_by"`
}

// AppCredential is an OAuth client_credentials identity for one App.
// ClientSecretHash stores only the hash of the one-time raw secret shown to admins.
type AppCredential struct {
	BaseModel
	AppID            string     `gorm:"size:40;index;not null" json:"app_id"`
	Name             string     `gorm:"size:255;not null" json:"name"`
	ClientID         string     `gorm:"size:128;uniqueIndex;not null" json:"client_id"`
	ClientSecretHash string     `gorm:"size:128;not null" json:"-"`
	Status           string     `gorm:"size:32;index;not null;default:active" json:"status"`
	Scopes           string     `gorm:"type:text" json:"scopes"`
	LastUsedAt       *time.Time `json:"last_used_at"`
	ExpiresAt        *time.Time `json:"expires_at"`
	CreatedBy        string     `gorm:"size:40;index" json:"created_by"`
}

// AppSession stores issued app token identifiers so tokens can be revoked and
// refresh tokens can be rotated.
type AppSession struct {
	BaseModel
	AppID            string     `gorm:"size:40;index;not null" json:"app_id"`
	CredentialID     string     `gorm:"size:40;index;not null" json:"credential_id"`
	AccessTokenHash  string     `gorm:"size:128;uniqueIndex;not null" json:"-"`
	RefreshTokenHash string     `gorm:"size:128;uniqueIndex;not null" json:"-"`
	ExpiresAt        time.Time  `gorm:"index;not null" json:"expires_at"`
	RevokedAt        *time.Time `json:"revoked_at"`
}

// ProviderAccount stores an encrypted provider configuration and its runtime
// activation state.
type ProviderAccount struct {
	BaseModel
	ProviderCode        string         `gorm:"size:80;index" json:"provider_code"`
	Name                string         `gorm:"size:255;not null" json:"name"`
	Environment         string         `gorm:"size:32;not null;default:sandbox" json:"environment"`
	Country             string         `gorm:"size:2;index;not null" json:"country"`
	Currency            string         `gorm:"size:3;index;not null;default:UGX" json:"currency"`
	Charges             ChargeSchedule `gorm:"embedded" json:"charges"`
	Active              bool           `gorm:"index;not null;default:false" json:"active"`
	ConfigVersion       int            `gorm:"not null;default:1" json:"config_version"`
	EncryptedConfigJSON string         `gorm:"type:text;not null" json:"-"`
	ConfigHash          string         `gorm:"size:128" json:"config_hash"`
}

// ProviderHealthSnapshot stores the latest observed provider and circuit state.
type ProviderHealthSnapshot struct {
	ProviderAccountID      string     `gorm:"primaryKey;size:40" json:"provider_account_id"`
	ProviderName           string     `gorm:"-" json:"provider_name"`
	Status                 string     `gorm:"size:32;index;not null;default:unknown" json:"status"`
	CircuitState           string     `gorm:"size:32;not null;default:closed" json:"circuit_state"`
	LastCheckedAt          *time.Time `json:"last_checked_at"`
	LastSuccessAt          *time.Time `json:"last_success_at"`
	LastFailureAt          *time.Time `json:"last_failure_at"`
	ConsecutiveFailures    int        `gorm:"not null;default:0" json:"consecutive_failures"`
	LatencyMs              int        `gorm:"not null;default:0" json:"latency_ms"`
	CollectionsAvailable   bool       `gorm:"not null;default:false" json:"collections_available"`
	DisbursementsAvailable bool       `gorm:"not null;default:false" json:"disbursements_available"`
	BalanceQueryAvailable  bool       `gorm:"not null;default:false" json:"balance_query_available"`
	LastErrorCode          string     `gorm:"size:128" json:"last_error_code"`
	LastErrorMessage       string     `gorm:"type:text" json:"last_error_message"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

// PaymentRoute assigns a payment service and method to a provider by priority.
type PaymentRoute struct {
	BaseModel
	ServiceType       string        `gorm:"size:64;uniqueIndex:idx_route_service_method_provider;not null" json:"service_type"`
	PaymentMethod     PaymentMethod `gorm:"size:64;uniqueIndex:idx_route_service_method_provider;not null" json:"payment_method"`
	ProviderAccountID string        `gorm:"size:40;uniqueIndex:idx_route_service_method_provider;not null" json:"provider_account_id"`
	ProviderName      string        `gorm:"-" json:"provider_name"`
	Priority          int           `gorm:"index;not null;default:100" json:"priority"`
	Active            bool          `gorm:"index;not null;default:true" json:"active"`
}

// Transaction records a collection or disbursement and its current outcome.
type Transaction struct {
	BaseModel
	AppID                     string        `gorm:"size:40;uniqueIndex:idx_tx_app_idempotency;uniqueIndex:idx_tx_app_reference;index;not null" json:"app_id"`
	ServiceType               string        `gorm:"size:64;index;not null" json:"service_type"`
	PaymentMethod             PaymentMethod `gorm:"size:64;index;not null" json:"payment_method"`
	Amount                    int64         `gorm:"not null" json:"amount"`
	Currency                  string        `gorm:"size:3;index;not null" json:"currency"`
	Country                   string        `gorm:"size:2;index;not null" json:"country,omitempty"`
	Reference                 string        `gorm:"size:128;uniqueIndex:idx_tx_app_reference;not null" json:"reference"`
	IdempotencyKey            string        `gorm:"size:255;uniqueIndex:idx_tx_app_idempotency;not null" json:"idempotency_key"`
	Status                    string        `gorm:"size:64;index;not null" json:"status"`
	SelectedRouteID           string        `gorm:"size:40;index" json:"selected_route_id"`
	SelectedProviderAccountID string        `gorm:"size:40;index" json:"selected_provider_account_id"`
	ProviderReference         string        `gorm:"size:255;index" json:"provider_reference"`
	CustomerAccount           string        `gorm:"size:255" json:"customer_account"`
	CustomerEmail             string        `gorm:"size:255" json:"customer_email"`
	CustomerName              string        `gorm:"size:255" json:"customer_name"`
	Description               string        `gorm:"type:text" json:"description"`
	ProviderFee               int64         `gorm:"not null;default:0" json:"provider_fee"`
	PlatformFee               int64         `gorm:"not null;default:0" json:"platform_fee"`
	RequestHash               string        `gorm:"size:128" json:"-"`
	ReconciliationAttempts    int           `gorm:"not null;default:0" json:"reconciliation_attempts"`
	LastReconciledAt          *time.Time    `json:"last_reconciled_at"`
	NextReconcileAt           *time.Time    `gorm:"index" json:"next_reconcile_at"`
}

// TransactionAttempt records one provider call made for a transaction.
type TransactionAttempt struct {
	BaseModel
	TransactionID     string     `gorm:"size:40;index;not null" json:"transaction_id"`
	ProviderAccountID string     `gorm:"size:40;index;not null" json:"provider_account_id"`
	ProviderCode      string     `gorm:"size:80;index" json:"provider_code"`
	ProviderReference string     `gorm:"size:255;index" json:"provider_reference"`
	Status            string     `gorm:"size:64;index;not null" json:"status"`
	RequestHash       string     `gorm:"size:128" json:"request_hash"`
	ErrorCode         string     `gorm:"size:128" json:"error_code"`
	ErrorMessage      string     `gorm:"type:text" json:"error_message"`
	RawResponse       string     `gorm:"type:text" json:"raw_response"`
	StartedAt         time.Time  `json:"started_at"`
	CompletedAt       *time.Time `json:"completed_at"`
}

// WebhookEvent stores a deduplicated provider callback for later processing.
type WebhookEvent struct {
	BaseModel
	ProviderAccountID string `gorm:"size:40;uniqueIndex:idx_webhook_provider_payload;not null" json:"provider_account_id"`
	ProviderReference string `gorm:"size:255;index" json:"provider_reference"`
	TransactionID     string `gorm:"size:40;index" json:"transaction_id"`
	EventType         string `gorm:"size:128;index;not null" json:"event_type"`
	PayloadHash       string `gorm:"size:128;uniqueIndex:idx_webhook_provider_payload;not null" json:"payload_hash"`
	Processed         bool   `gorm:"index;not null;default:false" json:"processed"`
	PayloadJSON       string `gorm:"type:text" json:"payload_json"`
}
