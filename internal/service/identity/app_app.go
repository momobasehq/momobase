package identity

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/repository"
	"github.com/momobasehq/momobase/internal/service/audit"
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
	repos *repository.UnitOfWork
	auth  *AppAuthService
	audit *audit.Service
}

// NewAppService creates an application management service.
func NewAppService(repos *repository.UnitOfWork, auth *AppAuthService, audit *audit.Service) *AppService {
	return &AppService{repos, auth, audit}
}
func (s *AppService) auditChange(ctx context.Context, actor *domain.AdminUser, action, resource, id string, meta map[string]any) {
	if actor != nil {
		s.audit.RecordBestEffort(ctx, actor.ID, "admin", action, resource, id, meta, "", "")
	}
}

// GetApp retrieves an application by ID.
func (s *AppService) GetApp(ctx context.Context, id string) (*domain.App, error) {
	return s.repos.Apps.ByID(ctx, id)
}

// CreateApp validates and persists an active application.
func (s *AppService) CreateApp(ctx context.Context, actor *domain.AdminUser, name, description, env string) (*domain.App, error) {
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
	err := s.repos.Apps.Create(ctx, app)
	if err == nil {
		s.auditChange(ctx, actor, "app.created", "app", app.ID, map[string]any{"name": name})
	}
	return app, err
}

// UpdateApp validates and applies mutable application attributes, then returns the updated application.
func (s *AppService) UpdateApp(ctx context.Context, actor *domain.AdminUser, id, name, description, env string) (*domain.App, error) {
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
	if err := s.repos.Apps.Update(ctx, id, updates); err != nil {
		return nil, err
	}
	s.auditChange(ctx, actor, "app.updated", "app", id, updates)
	return s.GetApp(ctx, id)
}

// ChangeAppStatus updates an application's status and revokes its sessions when it is no longer active.
func (s *AppService) ChangeAppStatus(ctx context.Context, actor *domain.AdminUser, id, status string) error {
	if status != "active" && status != "disabled" && status != "suspended" {
		return errors.New("invalid app status")
	}
	err := s.repos.Within(ctx, func(r *repository.Set) error {
		if err := r.Apps.SetStatus(ctx, id, status); err != nil {
			return err
		}
		if status == "active" {
			return nil
		}
		return r.AppSessions.RevokeForApp(ctx, id, time.Now().UTC())
	})
	if err == nil {
		s.auditChange(ctx, actor, "app.status_changed", "app", id, map[string]any{"status": status})
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
func (s *AppService) CreateCredential(
	ctx context.Context,
	actor *domain.AdminUser,
	appID, name, scopes string,
	expires *time.Time,
) (*CreatedCredential, error) {
	if _, err := s.GetApp(ctx, appID); err != nil {
		return nil, err
	}
	if name == "" {
		name = "Default credential"
	}
	if scopes == "" {
		scopes = defaultScopes
	}
	// Scopes are checked against the seeded app-audience catalogue, so a typo fails
	// here rather than surfacing as a 403 on the first payment the credential makes.
	scopes, err := ValidateAppScopes(scopes)
	if err != nil {
		return nil, err
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
	if err = s.repos.AppCredentials.Create(ctx, &cred); err != nil {
		return nil, err
	}
	s.auditChange(ctx, actor, "app_credential.created", "app_credential", cred.ID, map[string]any{"app_id": appID})
	return &CreatedCredential{cred, secret}, nil
}

// RevokeCredential revokes an application credential and all sessions issued through it.
func (s *AppService) RevokeCredential(ctx context.Context, actor *domain.AdminUser, appID, id string) error {
	// Revoking the credential and its live sessions is one transaction: a credential
	// marked revoked while its sessions survived would still authorize requests.
	err := s.repos.Within(ctx, func(r *repository.Set) error {
		if err := r.AppCredentials.Revoke(ctx, appID, id); err != nil {
			return err
		}
		return r.AppSessions.RevokeForCredential(ctx, id, time.Now().UTC())
	})
	if err == nil {
		s.auditChange(ctx, actor, "app_credential.revoked", "app_credential", id, map[string]any{"app_id": appID})
	}
	return err
}

// RotateCredential replaces an application credential's secret, reactivates it, and revokes its existing sessions.
func (s *AppService) RotateCredential(ctx context.Context, actor *domain.AdminUser, appID, id string) (*CreatedCredential, error) {
	cred, err := s.repos.AppCredentials.InApp(ctx, appID, id)
	if err != nil {
		return nil, repository.ErrNotFound
	}
	secret, hash, err := s.newSecret()
	if err != nil {
		return nil, err
	}
	err = s.repos.Within(ctx, func(r *repository.Set) error {
		if err := r.AppCredentials.Rotate(ctx, id, hash); err != nil {
			return err
		}
		return r.AppSessions.RevokeForCredential(ctx, id, time.Now().UTC())
	})
	if err != nil {
		return nil, err
	}
	cred.ClientSecretHash, cred.Status = "", "active"
	s.auditChange(ctx, actor, "app_credential.rotated", "app_credential", id, map[string]any{"app_id": appID})
	return &CreatedCredential{*cred, secret}, nil
}
