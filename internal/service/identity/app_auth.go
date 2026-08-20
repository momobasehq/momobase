package identity

import (
	"context"
	"errors"
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
func (s *AppAuthService) issue(ctx context.Context, id *AppIdentity, session *domain.AppSession) (*TokenResponse, error) {
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
	err = store.Within(ctx, s.db, func(db *gorm.DB) error {
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
func (s *AppAuthService) IssueClientToken(ctx context.Context, clientID, secret string) (*TokenResponse, error) {
	id, err := s.ValidateClientCredentials(ctx, clientID, secret)
	if err != nil {
		return nil, err
	}
	return s.issue(ctx, id, nil)
}

// RefreshToken validates an application refresh token and rotates its session token pair.
func (s *AppAuthService) RefreshToken(ctx context.Context, raw string) (*TokenResponse, error) {
	claims, err := s.tokens.Verify(raw)
	if err != nil || claims.SubjectType != "app" || claims.TokenType != "refresh" {
		return nil, errors.New("invalid app refresh token")
	}
	var session domain.AppSession
	if s.db.WithContext(ctx).Where(
		"app_id = ? AND credential_id = ? AND refresh_token_hash = ? AND expires_at > ? AND revoked_at IS NULL",
		claims.SubjectID,
		claims.CredentialID,
		platform.SHA256Hex(claims.TokenID),
		time.Now().UTC(),
	).First(&session).Error != nil {
		return nil, errors.New("invalid app refresh session")
	}
	id, err := s.identity(ctx, claims.SubjectID, claims.CredentialID)
	if err != nil {
		return nil, err
	}
	return s.issue(ctx, id, &session)
}

// ValidateClientCredentials verifies an active client ID and secret and returns the associated identity.
func (s *AppAuthService) ValidateClientCredentials(ctx context.Context, clientID, secret string) (*AppIdentity, error) {
	if clientID == "" || secret == "" {
		return nil, errors.New("missing client credentials")
	}
	var cred domain.AppCredential
	if s.db.WithContext(ctx).Where("client_id = ? AND status = ?", clientID, "active").
		First(&cred).Error != nil || !platform.VerifyPassword(cred.ClientSecretHash, secret) {
		return nil, errors.New("invalid client credentials")
	}
	return s.fromCredential(ctx, &cred)
}

// Verify authenticates an application token's signature and lifetime, without resolving
// the identity behind it. The HTTP layer verifies once per request and hands the claims
// to AuthenticateClaims.
func (s *AppAuthService) Verify(raw string) (*platform.TokenClaims, error) {
	return s.tokens.Verify(raw)
}

// AuthenticateBearer validates an application access token and returns its active identity.
func (s *AppAuthService) AuthenticateBearer(ctx context.Context, raw string) (*AppIdentity, error) {
	claims, err := s.tokens.Verify(raw)
	if err != nil {
		return nil, errors.New("invalid app token")
	}
	return s.AuthenticateClaims(ctx, claims)
}

// AuthenticateClaims resolves the active application identity behind an already verified
// token. The credential and its scopes are re-read from the database on every request, so
// a revoked credential stops working immediately rather than at the next refresh.
func (s *AppAuthService) AuthenticateClaims(
	ctx context.Context,
	claims *platform.TokenClaims,
) (*AppIdentity, error) {
	if claims == nil || claims.SubjectType != "app" || claims.TokenType != "access" {
		return nil, errors.New("invalid app token")
	}
	var session domain.AppSession
	if s.db.WithContext(ctx).Where(
		"app_id = ? AND credential_id = ? AND access_token_hash = ? AND expires_at > ? AND revoked_at IS NULL",
		claims.SubjectID,
		claims.CredentialID,
		platform.SHA256Hex(claims.TokenID),
		time.Now().UTC(),
	).First(&session).Error != nil {
		return nil, errors.New("invalid app session")
	}
	return s.identity(ctx, claims.SubjectID, claims.CredentialID)
}
func (s *AppAuthService) identity(ctx context.Context, appID, credentialID string) (*AppIdentity, error) {
	var cred domain.AppCredential
	if appID == "" || credentialID == "" ||
		s.db.WithContext(ctx).Where("id = ? AND app_id = ? AND status = ?", credentialID, appID, "active").
			First(&cred).Error != nil {
		return nil, errors.New("app credential inactive")
	}
	return s.fromCredential(ctx, &cred)
}
func (s *AppAuthService) fromCredential(ctx context.Context, cred *domain.AppCredential) (*AppIdentity, error) {
	if cred.Status != "active" || (cred.ExpiresAt != nil && !cred.ExpiresAt.After(time.Now().UTC())) {
		return nil, errors.New("app credential inactive or expired")
	}
	var app domain.App
	if s.db.WithContext(ctx).Where("id = ? AND status = ?", cred.AppID, "active").First(&app).Error != nil {
		return nil, errors.New("app inactive")
	}
	return &AppIdentity{app, *cred}, nil
}
