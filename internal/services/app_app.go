package services

import (
	"errors"
	"strings"
	"time"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/store"
	"gorm.io/gorm"
)

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
