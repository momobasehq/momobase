package services

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"momobase/internal/domain"
	"momobase/internal/platform"
	"momobase/internal/store"
)

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

func issueTokenPair(tokens *platform.TokenManager, claims platform.TokenClaims, accessTTL, refreshTTL time.Duration) (*TokenResponse, platform.TokenClaims, platform.TokenClaims, error) {
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

type AdminAuthService struct {
	db                    *gorm.DB
	accessTTL, refreshTTL time.Duration
	audit                 *AuditService
	tokens                *platform.TokenManager
}

func NewAdminAuthService(db *gorm.DB, accessTTL, refreshTTL time.Duration, audit *AuditService, tokens *platform.TokenManager) *AdminAuthService {
	return &AdminAuthService{db, accessTTL, refreshTTL, audit, tokens}
}
func (s *AdminAuthService) issue(user *domain.AdminUser, session *domain.AdminSession, ip, ua string) (*TokenResponse, error) {
	base := platform.TokenClaims{SubjectType: "admin", SubjectID: user.ID, Email: user.Email, Role: user.Role}
	response, ac, rc, err := issueTokenPair(s.tokens, base, s.accessTTL, s.refreshTTL)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	values := domain.AdminSession{BaseModel: domain.BaseModel{ID: platform.NewID("sess")}, AdminUserID: user.ID, TokenHash: platform.SHA256Hex(ac.TokenID), RefreshTokenHash: platform.SHA256Hex(rc.TokenID), IPAddress: ip, UserAgent: ua, ExpiresAt: now.Add(s.refreshTTL)}
	if session == nil {
		err = s.db.Create(&values).Error
	} else {
		err = store.Affected(s.db.Model(&domain.AdminSession{}).Where("id = ? AND refresh_token_hash = ?", session.ID, session.RefreshTokenHash).Updates(map[string]any{"token_hash": values.TokenHash, "refresh_token_hash": values.RefreshTokenHash, "ip_address": ip, "user_agent": ua, "expires_at": values.ExpiresAt}))
	}
	return response, err
}
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
	if err := s.db.Model(&user).Updates(map[string]any{"failed_login_attempts": 0, "locked_until": nil, "last_login_at": &now}).Error; err != nil {
		return nil, err
	}
	s.audit.RecordBestEffort(user.ID, "admin", "admin.login", "admin_user", user.ID, nil, ip, ua)
	return s.issue(&user, nil, ip, ua)
}
func (s *AdminAuthService) RefreshToken(raw, ip, ua string) (*TokenResponse, error) {
	claims, err := s.tokens.Verify(raw)
	if err != nil || claims.SubjectType != "admin" || claims.TokenType != "refresh" {
		return nil, errors.New("invalid refresh token")
	}
	var session domain.AdminSession
	now := time.Now().UTC()
	if s.db.Where("admin_user_id = ? AND refresh_token_hash = ? AND expires_at > ? AND revoked_at IS NULL", claims.SubjectID, platform.SHA256Hex(claims.TokenID), now).First(&session).Error != nil {
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
func (s *AdminAuthService) AuthenticateBearer(raw string) (*domain.AdminUser, error) {
	claims, err := s.tokens.Verify(raw)
	if err != nil || claims.SubjectType != "admin" || claims.TokenType != "access" {
		return nil, errors.New("invalid admin token")
	}
	var session domain.AdminSession
	if s.db.Where("admin_user_id = ? AND token_hash = ? AND expires_at > ? AND revoked_at IS NULL", claims.SubjectID, platform.SHA256Hex(claims.TokenID), time.Now().UTC()).First(&session).Error != nil {
		return nil, errors.New("invalid admin session")
	}
	return s.activeUser(claims.SubjectID)
}
func (s *AdminAuthService) LogoutBearer(raw, ip, ua string) error {
	claims, err := s.tokens.Verify(raw)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	err = s.db.Model(&domain.AdminSession{}).Where("admin_user_id = ? AND token_hash = ? AND revoked_at IS NULL", claims.SubjectID, platform.SHA256Hex(claims.TokenID)).Update("revoked_at", &now).Error
	if err == nil {
		s.audit.RecordBestEffort(claims.SubjectID, "admin", "admin.logout", "admin_user", claims.SubjectID, nil, ip, ua)
	}
	return err
}

type AdminUserService struct {
	db    *gorm.DB
	audit *AuditService
}

func NewAdminUserService(db *gorm.DB, audit *AuditService) *AdminUserService {
	return &AdminUserService{db, audit}
}
func (s *AdminUserService) Create(actor *domain.AdminUser, name, email, password, role string) (*domain.AdminUser, error) {
	if actor != nil && actor.Role != "super_admin" {
		return nil, errors.New("only super_admin can create admins")
	}
	name, email, role = strings.TrimSpace(name), strings.ToLower(strings.TrimSpace(email)), strings.TrimSpace(role)
	if role == "" {
		role = "operations"
	}
	if name == "" || len(name) > 255 || len(password) < 8 || strings.Count(email, "@") != 1 || role != "super_admin" && role != "operations" {
		return nil, errors.New("invalid admin name, email, password, or role")
	}
	hash, err := platform.HashPassword(password)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	user := &domain.AdminUser{BaseModel: domain.BaseModel{ID: platform.NewID("admusr")}, Name: name, Email: email, PasswordHash: hash, Role: role, Status: "active", PasswordChangedAt: &now}
	actorID := "system"
	if actor != nil {
		actorID, user.CreatedBy = actor.ID, actor.ID
	}
	if err = s.db.Create(user).Error; err == nil {
		s.audit.RecordBestEffort(actorID, "admin", "admin.created", "admin_user", user.ID, map[string]any{"email": user.Email, "role": role}, "", "")
	}
	return user, err
}
func (s *AdminUserService) ChangePassword(actor *domain.AdminUser, id, password string) error {
	if actor == nil || actor.Role != "super_admin" && actor.ID != id {
		return errors.New("not allowed")
	}
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	hash, err := platform.HashPassword(password)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := store.Affected(tx.Model(&domain.AdminUser{}).Where("id = ?", id).Updates(map[string]any{"password_hash": hash, "password_changed_at": &now})); err != nil {
			return err
		}
		return tx.Model(&domain.AdminSession{}).Where("admin_user_id = ? AND revoked_at IS NULL", id).Update("revoked_at", &now).Error
	})
	if err == nil {
		s.audit.RecordBestEffort(actor.ID, "admin", "admin.password_changed", "admin_user", id, nil, "", "")
	}
	return err
}
func (s *AdminUserService) ChangeStatus(actor *domain.AdminUser, id, status string) error {
	if actor == nil || actor.Role != "super_admin" {
		return errors.New("only super_admin can change status")
	}
	if status != "active" && status != "inactive" {
		return errors.New("invalid status")
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := store.Affected(tx.Model(&domain.AdminUser{}).Where("id = ?", id).Update("status", status)); err != nil {
			return err
		}
		if status == "active" {
			return nil
		}
		now := time.Now().UTC()
		return tx.Model(&domain.AdminSession{}).Where("admin_user_id = ? AND revoked_at IS NULL", id).Update("revoked_at", &now).Error
	})
	if err == nil {
		s.audit.RecordBestEffort(actor.ID, "admin", "admin.status_changed", "admin_user", id, map[string]any{"status": status}, "", "")
	}
	return err
}
