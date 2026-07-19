package services

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/store"
	"gorm.io/gorm"
)

// AdminUserService manages administrator accounts, passwords, and account status.
type AdminUserService struct {
	db    *gorm.DB
	audit *AuditService
}

// NewAdminUserService creates an administrator account service.
func NewAdminUserService(db *gorm.DB, audit *AuditService) *AdminUserService {
	return &AdminUserService{db, audit}
}

// Create validates and persists an administrator account, subject to the actor's role.
func (s *AdminUserService) Create(ctx context.Context, actor *domain.AdminUser, name, email, password, role string) (*domain.AdminUser, error) {
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
	user := &domain.AdminUser{
		BaseModel:         domain.BaseModel{ID: platform.NewID("admusr")},
		Name:              name,
		Email:             email,
		PasswordHash:      hash,
		Role:              role,
		Status:            "active",
		PasswordChangedAt: &now,
	}
	actorID := "system"
	if actor != nil {
		actorID, user.CreatedBy = actor.ID, actor.ID
	}
	if err = s.db.WithContext(ctx).Create(user).Error; err == nil {
		s.audit.RecordBestEffort(
			ctx,
			actorID,
			"admin",
			"admin.created",
			"admin_user",
			user.ID,
			map[string]any{"email": user.Email, "role": role},
			"",
			"",
		)
	}
	return user, err
}

// ChangePassword replaces an administrator's password and revokes all of that user's active sessions.
func (s *AdminUserService) ChangePassword(ctx context.Context, actor *domain.AdminUser, id, password string) error {
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
	err = store.Within(ctx, s.db, func(tx *gorm.DB) error {
		if err := store.Affected(
			tx.Model(&domain.AdminUser{}).
				Where("id = ?", id).
				Updates(map[string]any{
					"password_hash":       hash,
					"password_changed_at": &now,
				}),
		); err != nil {
			return err
		}
		return tx.Model(&domain.AdminSession{}).Where("admin_user_id = ? AND revoked_at IS NULL", id).Update("revoked_at", &now).Error
	})
	if err == nil {
		s.audit.RecordBestEffort(ctx, actor.ID, "admin", "admin.password_changed", "admin_user", id, nil, "", "")
	}
	return err
}

// ChangeStatus activates or deactivates an administrator and revokes sessions when deactivating.
func (s *AdminUserService) ChangeStatus(ctx context.Context, actor *domain.AdminUser, id, status string) error {
	if actor == nil || actor.Role != "super_admin" {
		return errors.New("only super_admin can change status")
	}
	if status != "active" && status != "inactive" {
		return errors.New("invalid status")
	}
	err := store.Within(ctx, s.db, func(tx *gorm.DB) error {
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
		s.audit.RecordBestEffort(ctx, actor.ID, "admin", "admin.status_changed", "admin_user", id, map[string]any{"status": status}, "", "")
	}
	return err
}
