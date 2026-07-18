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

// TokenResponse contains an issued OAuth-style access and refresh token pair.
type TokenResponse struct {
	// AccessToken is the signed bearer token used to authorize API requests.
	AccessToken string `json:"access_token"`
	// RefreshToken is the signed token used to obtain a replacement token pair.
	RefreshToken string `json:"refresh_token"`
	// TokenType identifies how the access token is presented to the API.
	TokenType string `json:"token_type"`
	// ExpiresIn is the access token lifetime in seconds.
	ExpiresIn int64 `json:"expires_in"`
}

func issueTokenPair(
	tokens *platform.TokenManager,
	claims platform.TokenClaims,
	accessTTL time.Duration,
	refreshTTL time.Duration,
) (*TokenResponse, platform.TokenClaims, platform.TokenClaims, error) {
	claims.TokenType = "access"
	access, ac, err := tokens.Issue(claims, accessTTL)
	if err != nil {
		return nil, ac, platform.TokenClaims{}, err
	}
	claims.TokenType = "refresh"
	refresh, rc, err := tokens.Issue(claims, refreshTTL)
	if err != nil {
		return nil, ac, rc, err
	}
	return &TokenResponse{access, refresh, "Bearer", int64(accessTTL.Seconds())}, ac, rc, nil
}

// AdminAuthService authenticates administrators and manages their token-backed sessions.
type AdminAuthService struct {
	db                    *gorm.DB
	accessTTL, refreshTTL time.Duration
	audit                 *AuditService
	tokens                *platform.TokenManager
}

// NewAdminAuthService creates an administrator authentication service with the supplied token lifetimes.
func NewAdminAuthService(
	db *gorm.DB,
	accessTTL time.Duration,
	refreshTTL time.Duration,
	audit *AuditService,
	tokens *platform.TokenManager,
) *AdminAuthService {
	return &AdminAuthService{db, accessTTL, refreshTTL, audit, tokens}
}
func (s *AdminAuthService) issue(user *domain.AdminUser, session *domain.AdminSession, ip, ua string) (*TokenResponse, error) {
	base := platform.TokenClaims{SubjectType: "admin", SubjectID: user.ID, Email: user.Email, Role: user.Role}
	response, ac, rc, err := issueTokenPair(s.tokens, base, s.accessTTL, s.refreshTTL)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	values := domain.AdminSession{
		BaseModel:        domain.BaseModel{ID: platform.NewID("sess")},
		AdminUserID:      user.ID,
		TokenHash:        platform.SHA256Hex(ac.TokenID),
		RefreshTokenHash: platform.SHA256Hex(rc.TokenID),
		IPAddress:        ip,
		UserAgent:        ua,
		ExpiresAt:        now.Add(s.refreshTTL),
	}
	if session == nil {
		err = s.db.Create(&values).Error
	} else {
		err = store.Affected(
			s.db.Model(&domain.AdminSession{}).
				Where(
					"id = ? AND refresh_token_hash = ?",
					session.ID,
					session.RefreshTokenHash,
				).
				Updates(map[string]any{
					"token_hash":         values.TokenHash,
					"refresh_token_hash": values.RefreshTokenHash,
					"ip_address":         ip,
					"user_agent":         ua,
					"expires_at":         values.ExpiresAt,
				}),
		)
	}
	return response, err
}

// IssuePasswordToken validates administrator credentials and issues a new token pair and session.
func (s *AdminAuthService) IssuePasswordToken(email, password, ip, ua string) (*TokenResponse, error) {
	var user domain.AdminUser
	if s.db.Where("email = ?", strings.ToLower(strings.TrimSpace(email))).First(&user).Error != nil || user.Status != "active" {
		return nil, errors.New("invalid credentials")
	}
	now := time.Now().UTC()
	if user.LockedUntil != nil && user.LockedUntil.After(now) {
		return nil, errors.New("admin account is locked")
	}
	if !platform.VerifyPassword(user.PasswordHash, password) {
		updates := map[string]any{"failed_login_attempts": gorm.Expr("failed_login_attempts + 1")}
		if user.FailedLoginAttempts >= 4 {
			updates["locked_until"] = now.Add(15 * time.Minute)
		}
		_ = s.db.Model(&user).Updates(updates).Error
		return nil, errors.New("invalid credentials")
	}
	if err := s.db.Model(&user).Updates(map[string]any{
		"failed_login_attempts": 0,
		"locked_until":          nil,
		"last_login_at":         &now,
	}).Error; err != nil {
		return nil, err
	}
	s.audit.RecordBestEffort(user.ID, "admin", "admin.login", "admin_user", user.ID, nil, ip, ua)
	return s.issue(&user, nil, ip, ua)
}

// RefreshToken validates an administrator refresh token and rotates its session token pair.
func (s *AdminAuthService) RefreshToken(raw, ip, ua string) (*TokenResponse, error) {
	claims, err := s.tokens.Verify(raw)
	if err != nil || claims.SubjectType != "admin" || claims.TokenType != "refresh" {
		return nil, errors.New("invalid refresh token")
	}
	var session domain.AdminSession
	now := time.Now().UTC()
	if s.db.Where(
		"admin_user_id = ? AND refresh_token_hash = ? AND expires_at > ? AND revoked_at IS NULL",
		claims.SubjectID,
		platform.SHA256Hex(claims.TokenID),
		now,
	).First(&session).Error != nil {
		return nil, errors.New("invalid refresh session")
	}
	user, err := s.activeUser(claims.SubjectID)
	if err != nil {
		return nil, err
	}
	return s.issue(user, &session, ip, ua)
}
func (s *AdminAuthService) activeUser(id string) (*domain.AdminUser, error) {
	var user domain.AdminUser
	if s.db.Where("id = ? AND status = ?", id, "active").First(&user).Error != nil {
		return nil, errors.New("admin inactive")
	}
	return &user, nil
}

// AuthenticateBearer validates an administrator access token and returns its active user.
func (s *AdminAuthService) AuthenticateBearer(raw string) (*domain.AdminUser, error) {
	claims, err := s.tokens.Verify(raw)
	if err != nil || claims.SubjectType != "admin" || claims.TokenType != "access" {
		return nil, errors.New("invalid admin token")
	}
	var session domain.AdminSession
	if s.db.Where(
		"admin_user_id = ? AND token_hash = ? AND expires_at > ? AND revoked_at IS NULL",
		claims.SubjectID,
		platform.SHA256Hex(claims.TokenID),
		time.Now().UTC(),
	).First(&session).Error != nil {
		return nil, errors.New("invalid admin session")
	}
	return s.activeUser(claims.SubjectID)
}

// LogoutBearer revokes the active administrator session represented by a bearer token.
func (s *AdminAuthService) LogoutBearer(raw, ip, ua string) error {
	claims, err := s.tokens.Verify(raw)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	err = s.db.Model(&domain.AdminSession{}).
		Where(
			"admin_user_id = ? AND token_hash = ? AND revoked_at IS NULL",
			claims.SubjectID,
			platform.SHA256Hex(claims.TokenID),
		).
		Update("revoked_at", &now).Error
	if err == nil {
		s.audit.RecordBestEffort(claims.SubjectID, "admin", "admin.logout", "admin_user", claims.SubjectID, nil, ip, ua)
	}
	return err
}
