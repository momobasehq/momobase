package repository

import (
	"context"
	"time"

	"github.com/momobasehq/momobase/internal/domain"
)

// AppRepo reads and writes tenant applications.
type AppRepo struct{ base[domain.App] }

// Create stores a new application.
func (r AppRepo) Create(ctx context.Context, app *domain.App) error { return r.create(ctx, app) }

// ByID returns an application whatever its status.
func (r AppRepo) ByID(ctx context.Context, id string) (*domain.App, error) {
	return r.first(ctx, "id = ?", id)
}

// ActiveByID returns an application only while it is active, which is what an
// authenticated request resolves against: suspending an app stops its tokens working.
func (r AppRepo) ActiveByID(ctx context.Context, id string) (*domain.App, error) {
	return r.first(ctx, "id = ? AND status = ?", id, "active")
}

// Page returns one page of applications, newest first.
func (r AppRepo) Page(ctx context.Context, number, size int) (Page[domain.App], error) {
	return r.page(ctx, "created_at desc", number, size)
}

// Update applies the supplied changes to one application.
func (r AppRepo) Update(ctx context.Context, id string, values map[string]any) error {
	return r.update(ctx, values, "id = ?", id)
}

// SetStatus activates or suspends an application.
func (r AppRepo) SetStatus(ctx context.Context, id, status string) error {
	return r.update(ctx, map[string]any{"status": status}, "id = ?", id)
}

// AppCredentialRepo reads and writes application client credentials.
type AppCredentialRepo struct{ base[domain.AppCredential] }

// Create stores a new credential.
func (r AppCredentialRepo) Create(ctx context.Context, cred *domain.AppCredential) error {
	return r.create(ctx, cred)
}

// ActiveByClientID returns the active credential a client ID names, for the client
// credentials grant.
func (r AppCredentialRepo) ActiveByClientID(ctx context.Context, clientID string) (*domain.AppCredential, error) {
	return r.first(ctx, "client_id = ? AND status = ?", clientID, "active")
}

// ActiveInApp returns an active credential scoped to its application. Both halves are
// matched, so a credential id from one tenant cannot resolve under another.
func (r AppCredentialRepo) ActiveInApp(ctx context.Context, appID, id string) (*domain.AppCredential, error) {
	return r.first(ctx, "id = ? AND app_id = ? AND status = ?", id, appID, "active")
}

// InApp returns a credential scoped to its application, whatever its status.
func (r AppCredentialRepo) InApp(ctx context.Context, appID, id string) (*domain.AppCredential, error) {
	return r.first(ctx, "id = ? AND app_id = ?", id, appID)
}

// PageForApp returns one page of an application's credentials, newest first.
func (r AppCredentialRepo) PageForApp(
	ctx context.Context,
	appID string,
	number, size int,
) (Page[domain.AppCredential], error) {
	total, err := r.count(ctx, "app_id = ?", appID)
	if err != nil {
		return Page[domain.AppCredential]{}, err
	}
	items := []domain.AppCredential{}
	err = r.session(ctx).Where("app_id = ?", appID).Order("created_at desc").
		Limit(size).Offset((number - 1) * size).Find(&items).Error
	return Page[domain.AppCredential]{Items: items, Total: total}, err
}

// Revoke marks a credential revoked within its application.
func (r AppCredentialRepo) Revoke(ctx context.Context, appID, id string) error {
	return r.update(ctx, map[string]any{"status": "revoked"}, "id = ? AND app_id = ?", id, appID)
}

// Rotate replaces a credential's secret hash and reactivates it.
func (r AppCredentialRepo) Rotate(ctx context.Context, id, hash string) error {
	return r.update(ctx, map[string]any{"client_secret_hash": hash, "status": "active"}, "id = ?", id)
}

// RecordUse stamps when a credential last authenticated a request.
func (r AppCredentialRepo) RecordUse(ctx context.Context, id string, at time.Time) error {
	return r.update(ctx, map[string]any{"last_used_at": &at}, "id = ?", id)
}

// AppSessionRepo reads and writes application sessions.
type AppSessionRepo struct{ base[domain.AppSession] }

// Create stores a new session.
func (r AppSessionRepo) Create(ctx context.Context, session *domain.AppSession) error {
	return r.create(ctx, session)
}

// Live returns the unrevoked, unexpired session an access token names.
func (r AppSessionRepo) Live(
	ctx context.Context,
	appID, credentialID, accessHash string,
	now time.Time,
) (*domain.AppSession, error) {
	return r.first(
		ctx,
		"app_id = ? AND credential_id = ? AND access_token_hash = ? AND expires_at > ? AND revoked_at IS NULL",
		appID, credentialID, accessHash, now,
	)
}

// LiveByRefresh returns the unrevoked, unexpired session a refresh token names.
func (r AppSessionRepo) LiveByRefresh(
	ctx context.Context,
	appID, credentialID, refreshHash string,
	now time.Time,
) (*domain.AppSession, error) {
	return r.first(
		ctx,
		"app_id = ? AND credential_id = ? AND refresh_token_hash = ? AND expires_at > ? AND revoked_at IS NULL",
		appID, credentialID, refreshHash, now,
	)
}

// Rotate swaps a session's token hashes for a freshly issued pair. The old refresh
// hash is part of the match, so a replayed refresh token cannot rotate a session that
// has already moved on.
func (r AppSessionRepo) Rotate(
	ctx context.Context,
	id, previousRefreshHash string,
	session *domain.AppSession,
) error {
	return r.update(ctx, map[string]any{
		"access_token_hash":  session.AccessTokenHash,
		"refresh_token_hash": session.RefreshTokenHash,
		"expires_at":         session.ExpiresAt,
	}, "id = ? AND refresh_token_hash = ?", id, previousRefreshHash)
}

// RevokeForApp revokes every live session an application holds.
func (r AppSessionRepo) RevokeForApp(ctx context.Context, appID string, at time.Time) error {
	return r.touch(ctx, map[string]any{"revoked_at": &at}, "app_id = ? AND revoked_at IS NULL", appID)
}

// RevokeForCredential revokes every live session issued through one credential.
func (r AppSessionRepo) RevokeForCredential(ctx context.Context, credentialID string, at time.Time) error {
	return r.touch(ctx, map[string]any{"revoked_at": &at},
		"credential_id = ? AND revoked_at IS NULL", credentialID)
}

// DeleteExpired removes sessions that expired before cutoff, for the cleanup worker.
func (r AppSessionRepo) DeleteExpired(ctx context.Context, cutoff time.Time) (int64, error) {
	result := r.session(ctx).Where("expires_at < ?", cutoff).Delete(&domain.AppSession{})
	return result.RowsAffected, result.Error
}
