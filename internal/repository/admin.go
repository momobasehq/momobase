package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/momobasehq/momobase/internal/domain"
)

// AdminUserRepo reads and writes administrator accounts.
type AdminUserRepo struct{ base[domain.AdminUser] }

// Create stores a new administrator.
func (r AdminUserRepo) Create(ctx context.Context, user *domain.AdminUser) error {
	return r.create(ctx, user)
}

// ByEmail returns the administrator registered under email, whatever their status.
// The caller decides what an inactive account means for the operation in hand.
func (r AdminUserRepo) ByEmail(ctx context.Context, email string) (*domain.AdminUser, error) {
	return r.first(ctx, "email = ?", email)
}

// ActiveByID returns the administrator only while the account is active, which is what
// every per-request identity lookup wants: a deactivated account stops resolving.
func (r AdminUserRepo) ActiveByID(ctx context.Context, id string) (*domain.AdminUser, error) {
	return r.first(ctx, "id = ? AND status = ?", id, "active")
}

// Page returns one page of administrators, newest first.
func (r AdminUserRepo) Page(ctx context.Context, number, size int) (Page[domain.AdminUser], error) {
	return r.page(ctx, "created_at desc", number, size)
}

// CountWithRole reports how many administrators hold a role, which is what makes
// deleting a role in use refusable.
func (r AdminUserRepo) CountWithRole(ctx context.Context, role string) (int64, error) {
	return r.count(ctx, "role = ?", role)
}

// SetRole reassigns an administrator to a different role.
func (r AdminUserRepo) SetRole(ctx context.Context, id, role string) error {
	return r.update(ctx, map[string]any{"role": role}, "id = ?", id)
}

// SetStatus activates or deactivates an administrator.
func (r AdminUserRepo) SetStatus(ctx context.Context, id, status string) error {
	return r.update(ctx, map[string]any{"status": status}, "id = ?", id)
}

// SetPassword replaces the stored hash and stamps when it changed.
func (r AdminUserRepo) SetPassword(ctx context.Context, id, hash string, changedAt time.Time) error {
	return r.update(ctx, map[string]any{
		"password_hash":       hash,
		"password_changed_at": &changedAt,
	}, "id = ?", id)
}

// RecordFailedLogin counts one failed attempt and locks the account until lockUntil
// when the caller has decided the threshold is reached.
//
// The increment is an expression rather than a read-then-write so two attempts racing
// each other still count twice.
func (r AdminUserRepo) RecordFailedLogin(ctx context.Context, id string, lockUntil *time.Time) error {
	values := map[string]any{"failed_login_attempts": gorm.Expr("failed_login_attempts + 1")}
	if lockUntil != nil {
		values["locked_until"] = *lockUntil
	}
	return r.touch(ctx, values, "id = ?", id)
}

// RecordLogin clears the failure counter and lock, and stamps the login time.
func (r AdminUserRepo) RecordLogin(ctx context.Context, id string, at time.Time) error {
	return r.touch(ctx, map[string]any{
		"failed_login_attempts": 0,
		"locked_until":          nil,
		"last_login_at":         &at,
	}, "id = ?", id)
}

// AdminSessionRepo reads and writes administrator sessions.
type AdminSessionRepo struct{ base[domain.AdminSession] }

// Create stores a new session.
func (r AdminSessionRepo) Create(ctx context.Context, session *domain.AdminSession) error {
	return r.create(ctx, session)
}

// Live returns the unrevoked, unexpired session an access token names.
func (r AdminSessionRepo) Live(
	ctx context.Context,
	adminUserID, tokenHash string,
	now time.Time,
) (*domain.AdminSession, error) {
	return r.first(
		ctx,
		"admin_user_id = ? AND token_hash = ? AND expires_at > ? AND revoked_at IS NULL",
		adminUserID, tokenHash, now,
	)
}

// LiveByRefresh returns the unrevoked, unexpired session a refresh token names.
func (r AdminSessionRepo) LiveByRefresh(
	ctx context.Context,
	adminUserID, refreshHash string,
	now time.Time,
) (*domain.AdminSession, error) {
	return r.first(
		ctx,
		"admin_user_id = ? AND refresh_token_hash = ? AND expires_at > ? AND revoked_at IS NULL",
		adminUserID, refreshHash, now,
	)
}

// Rotate swaps a session's token hashes for a freshly issued pair.
//
// The old refresh hash is part of the match, so a replayed refresh token cannot rotate
// a session that has already moved on.
func (r AdminSessionRepo) Rotate(
	ctx context.Context,
	id, previousRefreshHash string,
	session *domain.AdminSession,
) error {
	return r.update(ctx, map[string]any{
		"token_hash":         session.TokenHash,
		"refresh_token_hash": session.RefreshTokenHash,
		"ip_address":         session.IPAddress,
		"user_agent":         session.UserAgent,
		"expires_at":         session.ExpiresAt,
	}, "id = ? AND refresh_token_hash = ?", id, previousRefreshHash)
}

// RevokeByToken revokes the live session an access token names. A token whose session
// is already revoked is not an error: the caller asked for it to be gone, and it is.
func (r AdminSessionRepo) RevokeByToken(
	ctx context.Context,
	adminUserID, tokenHash string,
	at time.Time,
) error {
	return r.touch(ctx, map[string]any{"revoked_at": &at},
		"admin_user_id = ? AND token_hash = ? AND revoked_at IS NULL", adminUserID, tokenHash)
}

// RevokeAllFor revokes every live session an administrator holds, which is what a
// password change or a deactivation has to do.
func (r AdminSessionRepo) RevokeAllFor(ctx context.Context, adminUserID string, at time.Time) error {
	return r.touch(ctx, map[string]any{"revoked_at": &at},
		"admin_user_id = ? AND revoked_at IS NULL", adminUserID)
}

// DeleteExpired removes sessions that expired before cutoff, for the cleanup worker.
func (r AdminSessionRepo) DeleteExpired(ctx context.Context, cutoff time.Time) (int64, error) {
	result := r.session(ctx).Where("expires_at < ?", cutoff).Delete(&domain.AdminSession{})
	return result.RowsAffected, result.Error
}

// AuditLogRepo appends and lists audit entries.
type AuditLogRepo struct{ base[domain.AuditLog] }

// Create appends one audit entry.
func (r AuditLogRepo) Create(ctx context.Context, entry *domain.AuditLog) error {
	return r.create(ctx, entry)
}

// Page returns one page of audit entries, newest first.
func (r AuditLogRepo) Page(ctx context.Context, number, size int) (Page[domain.AuditLog], error) {
	return r.page(ctx, "created_at desc", number, size)
}

// DeleteBefore removes entries older than cutoff, for the cleanup worker.
func (r AuditLogRepo) DeleteBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	result := r.session(ctx).Where("created_at < ?", cutoff).Delete(&domain.AuditLog{})
	return result.RowsAffected, result.Error
}
