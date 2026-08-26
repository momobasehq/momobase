package identity

import (
	"context"
	"errors"
	"time"

	"github.com/momobasehq/momobase/internal/cache"
	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/repository"
	"github.com/momobasehq/momobase/internal/service/audit"
	"github.com/momobasehq/momobase/internal/utils"
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
	cache cache.Store
}

// CreateAppInput contains the attributes persisted for a new application.
type CreateAppInput struct {
	Name        string
	Description string
	Environment string
	Currency    string
	Charges     *domain.ChargeSchedule
}

// UpdateAppInput contains mutable application attributes. Nil Charges leaves the
// existing schedule unchanged.
type UpdateAppInput struct {
	Name        string
	Description string
	Environment string
	Currency    string
	Charges     *domain.ChargeSchedule
}

// NewAppService creates an application management service.
func NewAppService(
	repos *repository.UnitOfWork,
	auth *AppAuthService,
	audit *audit.Service,
	stores ...cache.Store,
) *AppService {
	var store cache.Store
	if len(stores) > 0 {
		store = stores[0]
	}
	return &AppService{repos: repos, auth: auth, audit: audit, cache: store}
}
func (s *AppService) auditChange(ctx context.Context, actor *domain.AdminUser, action, resource, id string, meta map[string]any) {
	if actor != nil {
		s.audit.RecordBestEffort(ctx, actor.ID, "admin", action, resource, id, meta, "", "")
	}
}

// GetApp retrieves an application by ID.
func (s *AppService) GetApp(ctx context.Context, id string) (*domain.App, error) {
	key := appCacheKey(id)
	if app := cache.Get[domain.App](ctx, s.cache, key); app != nil {
		return app, nil
	}

	app, err := s.repos.Apps.ByID(ctx, id)
	if err != nil {
		return nil, err
	}
	cache.Set(ctx, s.cache, key, app)
	return app, nil
}

// CreateApp validates and persists an active application.
func (s *AppService) CreateApp(
	ctx context.Context,
	actor *domain.AdminUser,
	input CreateAppInput,
) (*domain.App, error) {
	if input.Environment == "" {
		input.Environment = "production"
	}
	currency, err := utils.NormalizeCurrency(input.Currency)
	if err != nil {
		return nil, err
	}
	charges := domain.ChargeSchedule{}
	if input.Charges == nil {
		charges.Normalize()
	} else {
		charges = *input.Charges
	}
	if err := charges.Validate(); err != nil {
		return nil, err
	}
	app := &domain.App{
		BaseModel:   domain.BaseModel{ID: platform.NewID("app")},
		Name:        input.Name,
		Description: input.Description,
		Environment: input.Environment,
		Currency:    currency,
		Charges:     charges,
		Status:      "active",
	}
	if actor != nil {
		app.CreatedBy = actor.ID
	}
	err = s.repos.Apps.Create(ctx, app)
	if err == nil {
		s.auditChange(ctx, actor, "app.created", "app", app.ID, map[string]any{
			"name": input.Name, "currency": currency, "charges": charges,
		})
		cache.Set(ctx, s.cache, appCacheKey(app.ID), app)
	}
	return app, err
}

// UpdateApp validates and applies mutable application attributes, then returns the updated application.
func (s *AppService) UpdateApp(
	ctx context.Context,
	actor *domain.AdminUser,
	id string,
	input UpdateAppInput,
) (*domain.App, error) {
	updates := map[string]any{"description": input.Description}
	if input.Name != "" {
		updates["name"] = input.Name
	}
	if input.Environment != "" {
		updates["environment"] = input.Environment
	}
	if input.Currency != "" {
		currency, err := utils.NormalizeCurrency(input.Currency)
		if err != nil {
			return nil, err
		}
		updates["currency"] = currency
	}
	if input.Charges != nil {
		if err := input.Charges.Validate(); err != nil {
			return nil, err
		}
		addChargeUpdates(updates, *input.Charges)
	}
	if err := s.repos.Apps.Update(ctx, id, updates); err != nil {
		return nil, err
	}
	s.auditChange(ctx, actor, "app.updated", "app", id, updates)
	app, err := s.repos.Apps.ByID(ctx, id)
	if err != nil {
		return nil, err
	}
	cache.Set(ctx, s.cache, appCacheKey(id), app)
	return app, nil
}

// ChangeAppStatus updates an application's status and revokes its sessions when it is no longer active.
func (s *AppService) ChangeAppStatus(ctx context.Context, actor *domain.AdminUser, id, status string) error {
	var app *domain.App
	err := s.repos.Within(ctx, func(r *repository.Set) error {
		if err := r.Apps.SetStatus(ctx, id, status); err != nil {
			return err
		}
		if status != "active" {
			if err := r.AppSessions.RevokeForApp(ctx, id, time.Now().UTC()); err != nil {
				return err
			}
		}
		var err error
		app, err = r.Apps.ByID(ctx, id)
		return err
	})
	if err != nil {
		return err
	}
	s.auditChange(ctx, actor, "app.status_changed", "app", id, map[string]any{"status": status})
	cache.Set(ctx, s.cache, appCacheKey(id), app)
	return nil
}

func appCacheKey(id string) string {
	return "app:v2:" + id
}

func addChargeUpdates(updates map[string]any, charges domain.ChargeSchedule) {
	updates["collection_charge_type"] = charges.Collection.Type
	updates["collection_charge_value"] = charges.Collection.Value
	updates["disbursement_charge_type"] = charges.Disbursement.Type
	updates["disbursement_charge_value"] = charges.Disbursement.Value
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
