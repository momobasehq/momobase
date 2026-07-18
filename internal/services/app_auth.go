package services

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/store"
)

const defaultScopes = "collections:create disbursements:create transactions:read"

// AppIdentity identifies an authenticated application and the credential used to authenticate it.
type AppIdentity struct {
	// App is the active application associated with the identity.
	App domain.App
	// Credential is the active credential used to establish the identity.
	Credential domain.AppCredential
}

// AppAuthService authenticates application credentials and manages app token sessions.
type AppAuthService struct {
	db                         *gorm.DB
	clientPrefix, secretPrefix string
	accessTTL, refreshTTL      time.Duration
	tokens                     *platform.TokenManager
}

// NewAppAuthService creates an application authentication service with the supplied credential prefixes and token lifetimes.
func NewAppAuthService(
	db *gorm.DB,
	clientPrefix string,
	secretPrefix string,
	accessTTL time.Duration,
	refreshTTL time.Duration,
	tokens *platform.TokenManager,
) *AppAuthService {
	return &AppAuthService{db, clientPrefix, secretPrefix, accessTTL, refreshTTL, tokens}
}
func (s *AppAuthService) claims(id *AppIdentity, kind string) platform.TokenClaims {
	return platform.TokenClaims{
		SubjectType:  "app",
		SubjectID:    id.App.ID,
		CredentialID: id.Credential.ID,
		Scopes:       id.Credential.Scopes,
		TokenType:    kind,
		Extra:        map[string]string{"client_id": id.Credential.ClientID},
	}
}
func (s *AppAuthService) issue(id *AppIdentity, session *domain.AppSession) (*TokenResponse, error) {
	response, ac, rc, err := issueTokenPair(s.tokens, s.claims(id, "access"), s.accessTTL, s.refreshTTL)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	values := domain.AppSession{
		BaseModel:        domain.BaseModel{ID: platform.NewID("appsess")},
		AppID:            id.App.ID,
		CredentialID:     id.Credential.ID,
		AccessTokenHash:  platform.SHA256Hex(ac.TokenID),
		RefreshTokenHash: platform.SHA256Hex(rc.TokenID),
		ExpiresAt:        now.Add(s.refreshTTL),
	}
	err = s.db.Transaction(func(db *gorm.DB) error {
		if session == nil {
			if err := db.Create(&values).Error; err != nil {
				return err
			}
		} else if err := store.Affected(
			db.Model(&domain.AppSession{}).
				Where(
					"id = ? AND refresh_token_hash = ?",
					session.ID,
					session.RefreshTokenHash,
				).
				Updates(map[string]any{
					"access_token_hash":  values.AccessTokenHash,
					"refresh_token_hash": values.RefreshTokenHash,
					"expires_at":         values.ExpiresAt,
				}),
		); err != nil {
			return err
		}
		return store.Affected(db.Model(&domain.AppCredential{}).Where("id = ?", id.Credential.ID).Update("last_used_at", &now))
	})
	return response, err
}

// IssueClientToken validates client credentials and issues a new application token pair and session.
func (s *AppAuthService) IssueClientToken(clientID, secret string) (*TokenResponse, error) {
	id, err := s.ValidateClientCredentials(clientID, secret)
	if err != nil {
		return nil, err
	}
	return s.issue(id, nil)
}

// RefreshToken validates an application refresh token and rotates its session token pair.
func (s *AppAuthService) RefreshToken(raw string) (*TokenResponse, error) {
	claims, err := s.tokens.Verify(raw)
	if err != nil || claims.SubjectType != "app" || claims.TokenType != "refresh" {
		return nil, errors.New("invalid app refresh token")
	}
	var session domain.AppSession
	if s.db.Where(
		"app_id = ? AND credential_id = ? AND refresh_token_hash = ? AND expires_at > ? AND revoked_at IS NULL",
		claims.SubjectID,
		claims.CredentialID,
		platform.SHA256Hex(claims.TokenID),
		time.Now().UTC(),
	).First(&session).Error != nil {
		return nil, errors.New("invalid app refresh session")
	}
	id, err := s.identity(claims.SubjectID, claims.CredentialID)
	if err != nil {
		return nil, err
	}
	return s.issue(id, &session)
}

// ValidateClientCredentials verifies an active client ID and secret and returns the associated identity.
func (s *AppAuthService) ValidateClientCredentials(clientID, secret string) (*AppIdentity, error) {
	if clientID == "" || secret == "" {
		return nil, errors.New("missing client credentials")
	}
	var cred domain.AppCredential
	if s.db.Where("client_id = ? AND status = ?", clientID, "active").
		First(&cred).Error != nil || !platform.VerifyPassword(cred.ClientSecretHash, secret) {
		return nil, errors.New("invalid client credentials")
	}
	return s.fromCredential(&cred)
}

// AuthenticateBearer validates an application access token and returns its active identity.
func (s *AppAuthService) AuthenticateBearer(raw string) (*AppIdentity, error) {
	claims, err := s.tokens.Verify(raw)
	if err != nil || claims.SubjectType != "app" || claims.TokenType != "access" {
		return nil, errors.New("invalid app token")
	}
	var session domain.AppSession
	if s.db.Where(
		"app_id = ? AND credential_id = ? AND access_token_hash = ? AND expires_at > ? AND revoked_at IS NULL",
		claims.SubjectID,
		claims.CredentialID,
		platform.SHA256Hex(claims.TokenID),
		time.Now().UTC(),
	).First(&session).Error != nil {
		return nil, errors.New("invalid app session")
	}
	return s.identity(claims.SubjectID, claims.CredentialID)
}
func (s *AppAuthService) identity(appID, credentialID string) (*AppIdentity, error) {
	var cred domain.AppCredential
	if appID == "" || credentialID == "" ||
		s.db.Where("id = ? AND app_id = ? AND status = ?", credentialID, appID, "active").
			First(&cred).Error != nil {
		return nil, errors.New("app credential inactive")
	}
	return s.fromCredential(&cred)
}
func (s *AppAuthService) fromCredential(cred *domain.AppCredential) (*AppIdentity, error) {
	if cred.Status != "active" || (cred.ExpiresAt != nil && !cred.ExpiresAt.After(time.Now().UTC())) {
		return nil, errors.New("app credential inactive or expired")
	}
	var app domain.App
	if s.db.Where("id = ? AND status = ?", cred.AppID, "active").First(&app).Error != nil {
		return nil, errors.New("app inactive")
	}
	return &AppIdentity{app, *cred}, nil
}

// CreatedCredential contains a persisted application credential and its one-time plaintext secret.
type CreatedCredential struct {
	// Credential is the persisted credential metadata.
	Credential domain.AppCredential `json:"credential"`
	// ClientSecret is the newly generated plaintext secret returned only at creation or rotation.
	ClientSecret string `json:"client_secret"`
}

// AppService manages applications and their client credentials.
type AppService struct {
	db    *gorm.DB
	auth  *AppAuthService
	audit *AuditService
}

// NewAppService creates an application management service.
func NewAppService(db *gorm.DB, auth *AppAuthService, audit *AuditService) *AppService {
	return &AppService{db, auth, audit}
}
func (s *AppService) auditChange(actor *domain.AdminUser, action, resource, id string, meta map[string]any) {
	if actor != nil {
		s.audit.RecordBestEffort(actor.ID, "admin", action, resource, id, meta, "", "")
	}
}

// GetApp retrieves an application by ID.
func (s *AppService) GetApp(id string) (*domain.App, error) {
	var app domain.App
	return &app, s.db.First(&app, "id = ?", id).Error
}

// CreateApp validates and persists an active application.
func (s *AppService) CreateApp(actor *domain.AdminUser, name, description, env string) (*domain.App, error) {
	name, env = strings.TrimSpace(name), strings.ToLower(strings.TrimSpace(env))
	if env == "" {
		env = "production"
	}
	if name == "" || len(name) > 255 || len(description) > 1000 || env != "sandbox" && env != "production" {
		return nil, errors.New("invalid app name, description, or environment")
	}
	app := &domain.App{
		BaseModel:   domain.BaseModel{ID: platform.NewID("app")},
		Name:        name,
		Description: description,
		Environment: env,
		Status:      "active",
	}
	if actor != nil {
		app.CreatedBy = actor.ID
	}
	err := s.db.Create(app).Error
	if err == nil {
		s.auditChange(actor, "app.created", "app", app.ID, map[string]any{"name": name})
	}
	return app, err
}

// UpdateApp validates and applies mutable application attributes, then returns the updated application.
func (s *AppService) UpdateApp(actor *domain.AdminUser, id, name, description, env string) (*domain.App, error) {
	name, env = strings.TrimSpace(name), strings.ToLower(strings.TrimSpace(env))
	if len(name) > 255 || len(description) > 1000 || env != "" && env != "sandbox" && env != "production" {
		return nil, errors.New("invalid app name, description, or environment")
	}
	updates := map[string]any{"description": description}
	if name != "" {
		updates["name"] = name
	}
	if env != "" {
		updates["environment"] = env
	}
	if err := store.Affected(s.db.Model(&domain.App{}).Where("id = ?", id).Updates(updates)); err != nil {
		return nil, err
	}
	s.auditChange(actor, "app.updated", "app", id, updates)
	return s.GetApp(id)
}

// ChangeAppStatus updates an application's status and revokes its sessions when it is no longer active.
func (s *AppService) ChangeAppStatus(actor *domain.AdminUser, id, status string) error {
	if status != "active" && status != "disabled" && status != "suspended" {
		return errors.New("invalid app status")
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := store.Affected(tx.Model(&domain.App{}).Where("id = ?", id).Update("status", status)); err != nil {
			return err
		}
		if status == "active" {
			return nil
		}
		now := time.Now().UTC()
		return tx.Model(&domain.AppSession{}).Where("app_id = ? AND revoked_at IS NULL", id).Update("revoked_at", &now).Error
	})
	if err == nil {
		s.auditChange(actor, "app.status_changed", "app", id, map[string]any{"status": status})
	}
	return err
}
func (s *AppService) newSecret() (string, string, error) {
	secret, err := platform.SecureRandomToken(s.auth.secretPrefix, 32)
	if err != nil {
		return "", "", err
	}
	hash, err := platform.HashPassword(secret)
	return secret, hash, err
}

// CreateCredential generates and persists a credential for an existing application.
func (s *AppService) CreateCredential(actor *domain.AdminUser, appID, name, scopes string, expires *time.Time) (*CreatedCredential, error) {
	if _, err := s.GetApp(appID); err != nil {
		return nil, err
	}
	if name == "" {
		name = "Default credential"
	}
	if scopes == "" {
		scopes = defaultScopes
	}
	if len(name) > 255 || len(scopes) > 1000 {
		return nil, errors.New("credential name or scopes too long")
	}
	secret, hash, err := s.newSecret()
	if err != nil {
		return nil, err
	}
	cred := domain.AppCredential{
		BaseModel:        domain.BaseModel{ID: platform.NewID("cred")},
		AppID:            appID,
		Name:             name,
		ClientID:         platform.NewID(s.auth.clientPrefix),
		ClientSecretHash: hash,
		Status:           "active",
		Scopes:           scopes,
		ExpiresAt:        expires,
	}
	if actor != nil {
		cred.CreatedBy = actor.ID
	}
	if err = s.db.Create(&cred).Error; err != nil {
		return nil, err
	}
	s.auditChange(actor, "app_credential.created", "app_credential", cred.ID, map[string]any{"app_id": appID})
	return &CreatedCredential{cred, secret}, nil
}
func (s *AppService) revokeSessions(tx *gorm.DB, credentialID string) error {
	now := time.Now().UTC()
	return tx.Model(&domain.AppSession{}).Where("credential_id = ? AND revoked_at IS NULL", credentialID).Update("revoked_at", &now).Error
}

// RevokeCredential revokes an application credential and all sessions issued through it.
func (s *AppService) RevokeCredential(actor *domain.AdminUser, appID, id string) error {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := store.Affected(
			tx.Model(&domain.AppCredential{}).
				Where("id = ? AND app_id = ?", id, appID).
				Update("status", "revoked"),
		); err != nil {
			return err
		}
		return s.revokeSessions(tx, id)
	})
	if err == nil {
		s.auditChange(actor, "app_credential.revoked", "app_credential", id, map[string]any{"app_id": appID})
	}
	return err
}

// RotateCredential replaces an application credential's secret, reactivates it, and revokes its existing sessions.
func (s *AppService) RotateCredential(actor *domain.AdminUser, appID, id string) (*CreatedCredential, error) {
	var cred domain.AppCredential
	if s.db.Where("id = ? AND app_id = ?", id, appID).First(&cred).Error != nil {
		return nil, gorm.ErrRecordNotFound
	}
	secret, hash, err := s.newSecret()
	if err != nil {
		return nil, err
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := store.Affected(tx.Model(&cred).Updates(map[string]any{"client_secret_hash": hash, "status": "active"})); err != nil {
			return err
		}
		return s.revokeSessions(tx, id)
	})
	if err != nil {
		return nil, err
	}
	cred.ClientSecretHash, cred.Status = "", "active"
	s.auditChange(actor, "app_credential.rotated", "app_credential", id, map[string]any{"app_id": appID})
	return &CreatedCredential{cred, secret}, nil
}
