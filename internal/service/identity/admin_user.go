package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/service/audit"
	"github.com/momobasehq/momobase/internal/store"
	"gorm.io/gorm"
)

// AdminUserService manages administrator accounts, passwords, and account status.
type AdminUserService struct {
	db    *gorm.DB
	audit *audit.Service
	authz *AuthzService
}

// NewAdminUserService creates an administrator account service.
func NewAdminUserService(db *gorm.DB, audit *audit.Service, authz *AuthzService) *AdminUserService {
	return &AdminUserService{db, audit, authz}
}

// Create validates and persists an administrator account, subject to the actor's role.
func (s *AdminUserService) Create(ctx context.Context, actor *domain.AdminUser, name, email, password, role string) (*domain.AdminUser, error) {
	name, email, role = strings.TrimSpace(name), strings.ToLower(strings.TrimSpace(email)), strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		role = domain.RoleOperations
	}
	if name == "" || len(name) > 255 || len(password) < 8 || strings.Count(email, "@") != 1 {
		return nil, errors.New("invalid admin name, email, or password")
	}
	// Any seeded or operator-created role is assignable. Checking the roles table
	// rather than a literal list is the point: a custom role was unusable before,
	// because the two valid names were compiled in here.
	if s.authz != nil {
		exists, err := s.authz.RoleExists(ctx, role)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, fmt.Errorf("unknown role %q", role)
		}
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
	if actor != nil {
		user.CreatedBy = actor.ID
	}
	if err = s.db.WithContext(ctx).Create(user).Error; err == nil {
		s.audit.RecordBestEffort(
			ctx,
			actor.ActorID(),
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
	// Self-service is deliberately not a permission: changing your own password is
	// allowed without users:update, and changing someone else's needs it, which the
	// route's middleware has already enforced by the time this runs.
	if actor == nil {
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

// ChangeRole reassigns an administrator to a different role.
//
// No session is revoked, and none needs to be: permissions are resolved from the role
// on every request rather than carried in the access token, so the change takes effect
// on the target's very next call.
//
// Changing your own role is refused. It is the one case that is both a lockout risk —
// the last super_admin demoting itself leaves nobody able to put it back — and a
// privilege escalation, since users:update would otherwise be enough to promote
// yourself to super_admin.
func (s *AdminUserService) ChangeRole(ctx context.Context, actor *domain.AdminUser, id, role string) error {
	if actor == nil {
		return errors.New("not allowed")
	}
	if actor.ID == id {
		return errors.New("an administrator cannot change their own role; ask another administrator")
	}
	role = strings.ToLower(strings.TrimSpace(role))
	if s.authz != nil {
		exists, err := s.authz.RoleExists(ctx, role)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("unknown role %q", role)
		}
	}
	if err := store.Affected(
		s.db.WithContext(ctx).Model(&domain.AdminUser{}).Where("id = ?", id).Update("role", role),
	); err != nil {
		return err
	}
	s.audit.RecordBestEffort(ctx, actor.ID, "admin", "admin.role_changed", "admin_user", id, map[string]any{"role": role}, "", "")
	return nil
}

// ChangeStatus activates or deactivates an administrator and revokes sessions when deactivating.
func (s *AdminUserService) ChangeStatus(ctx context.Context, actor *domain.AdminUser, id, status string) error {
	if actor == nil {
		return errors.New("not allowed")
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
