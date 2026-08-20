package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/momobasehq/momobase/internal/domain"
)

// Set is one repository per persisted entity, all sharing a handle.
//
// A Set built by New queries the connection pool. A Set handed to Within's callback
// queries that transaction instead, which is what lets a service write several tables
// atomically without ever holding a database handle itself.
type Set struct {
	AdminUsers          AdminUserRepo
	AdminSessions       AdminSessionRepo
	Roles               RoleRepo
	Permissions         PermissionRepo
	AuditLogs           AuditLogRepo
	Apps                AppRepo
	AppCredentials      AppCredentialRepo
	AppSessions         AppSessionRepo
	ProviderAccounts    ProviderAccountRepo
	ProviderHealth      ProviderHealthRepo
	PaymentRoutes       PaymentRouteRepo
	Transactions        TransactionRepo
	TransactionAttempts TransactionAttemptRepo
	WebhookEvents       WebhookEventRepo
}

func newSet(db *gorm.DB) *Set {
	return &Set{
		AdminUsers:          AdminUserRepo{base: base[domain.AdminUser]{db: db}},
		AdminSessions:       AdminSessionRepo{base: base[domain.AdminSession]{db: db}},
		Roles:               RoleRepo{base: base[domain.Role]{db: db}, db: db},
		Permissions:         PermissionRepo{base: base[domain.Permission]{db: db}, db: db},
		AuditLogs:           AuditLogRepo{base: base[domain.AuditLog]{db: db}},
		Apps:                AppRepo{base: base[domain.App]{db: db}},
		AppCredentials:      AppCredentialRepo{base: base[domain.AppCredential]{db: db}},
		AppSessions:         AppSessionRepo{base: base[domain.AppSession]{db: db}},
		ProviderAccounts:    ProviderAccountRepo{base: base[domain.ProviderAccount]{db: db}},
		ProviderHealth:      ProviderHealthRepo{base: base[domain.ProviderHealthSnapshot]{db: db}, db: db},
		PaymentRoutes:       PaymentRouteRepo{base: base[domain.PaymentRoute]{db: db}},
		Transactions:        TransactionRepo{base: base[domain.Transaction]{db: db}, db: db},
		TransactionAttempts: TransactionAttemptRepo{base: base[domain.TransactionAttempt]{db: db}},
		WebhookEvents:       WebhookEventRepo{base: base[domain.WebhookEvent]{db: db}, db: db},
	}
}

// UnitOfWork owns the database handle and hands out repositories bound to it.
//
// It embeds a Set, so a service that only reads uses u.Apps directly and reaches for
// Within solely when a change has to be atomic.
type UnitOfWork struct {
	*Set
	db *gorm.DB
}

// New constructs the unit of work over an open database handle.
func New(db *gorm.DB) *UnitOfWork {
	return &UnitOfWork{Set: newSet(db), db: db}
}

// Within runs fn inside one database transaction, with every repository in the set it
// receives bound to that transaction. It is the only transaction boundary in Momobase.
//
// Nothing that talks to a provider belongs inside fn: a network call held open across a
// transaction holds a row lock for as long as the remote end takes to answer.
func (u *UnitOfWork) Within(ctx context.Context, fn func(*Set) error) error {
	return u.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(newSet(tx))
	})
}

// Ping reports whether the database answers, for the health endpoint.
func (u *UnitOfWork) Ping(ctx context.Context) error {
	pool, err := u.db.DB()
	if err != nil {
		return err
	}
	return pool.PingContext(ctx)
}

// Dialect names the driver in use, which the analytics bucketing needs because the
// date-truncation expression is not portable.
func (u *UnitOfWork) Dialect() string { return u.db.Name() }
