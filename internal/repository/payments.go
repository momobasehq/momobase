package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/momobasehq/momobase/internal/domain"
)

// ProviderAccountRepo reads and writes provider accounts and their encrypted config.
type ProviderAccountRepo struct{ base[domain.ProviderAccount] }

// Create stores a new provider account.
func (r ProviderAccountRepo) Create(ctx context.Context, account *domain.ProviderAccount) error {
	return r.create(ctx, account)
}

// ByID returns one provider account.
func (r ProviderAccountRepo) ByID(ctx context.Context, id string) (*domain.ProviderAccount, error) {
	return r.first(ctx, "id = ?", id)
}

// ActiveByID returns a provider account only while it is active, which is what route
// selection resolves against: deactivating an account takes its routes out of play.
func (r ProviderAccountRepo) ActiveByID(ctx context.Context, id string) (*domain.ProviderAccount, error) {
	return r.first(ctx, "id = ? AND active = ?", id, true)
}

// Active returns every active provider account, which is what the runtime loads.
func (r ProviderAccountRepo) Active(ctx context.Context) ([]domain.ProviderAccount, error) {
	return r.find(ctx, "", "active = ?", true)
}

// Exists reports whether a provider account of that id is defined, which is what makes
// a route pointing at nothing refusable.
func (r ProviderAccountRepo) Exists(ctx context.Context, id string) (bool, error) {
	count, err := r.count(ctx, "id = ?", id)
	return count == 1, err
}

// CountActive reports how many provider accounts are active, for the health endpoint.
func (r ProviderAccountRepo) CountActive(ctx context.Context) (int64, error) {
	return r.count(ctx, "active = ?", true)
}

var providerAccountList = listSpec{
	filters: map[string]filterSpec{
		"provider_code": {column: "provider_code"},
		"environment":   {column: "environment"},
		"currency":      {column: "currency"},
		"active":        {column: "active", kind: boolFilter},
	},
	search: []string{"name", "provider_code", "id"},
	sorts:  map[string]string{"name": "name", "provider_code": "provider_code", "updated_at": "updated_at", "created_at": "created_at"},
	order:  "created_at desc, id desc",
}

// Page returns one page of provider accounts, newest first unless q sorts otherwise.
func (r ProviderAccountRepo) Page(ctx context.Context, q ListQuery) (Page[domain.ProviderAccount], error) {
	return r.list(ctx, providerAccountList, q)
}

// NamesByIDs returns provider account display names keyed by ID. List endpoints use
// this to enrich rows in one query instead of looking up every provider separately.
func (r ProviderAccountRepo) NamesByIDs(ctx context.Context, ids []string) (map[string]string, error) {
	names := make(map[string]string, len(ids))
	if len(ids) == 0 {
		return names, nil
	}
	type row struct {
		ID   string
		Name string
	}
	rows := []row{}
	err := r.session(ctx).
		Model(&domain.ProviderAccount{}).
		Select("id", "name").
		Where("id IN ?", ids).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, item := range rows {
		names[item.ID] = item.Name
	}
	return names, nil
}

// Update applies the supplied changes to one provider account.
func (r ProviderAccountRepo) Update(ctx context.Context, id string, values map[string]any) error {
	return r.update(ctx, values, "id = ?", id)
}

// Restore reapplies previous values without requiring a match. It is the rollback half
// of a config change whose adapter reload failed, where the row must go back however
// the reload ended.
func (r ProviderAccountRepo) Restore(ctx context.Context, id string, values map[string]any) error {
	return r.touch(ctx, values, "id = ?", id)
}

// SetConfig replaces the encrypted configuration and bumps its version. The version is
// an expression rather than a read-then-write, so two changes racing each other still
// produce two versions.
func (r ProviderAccountRepo) SetConfig(ctx context.Context, id, cipher, hash string) error {
	return r.update(ctx, map[string]any{
		"encrypted_config_json": cipher,
		"config_hash":           hash,
		"config_version":        gorm.Expr("config_version + 1"),
	}, "id = ?", id)
}

// SetActive activates or deactivates a provider account.
func (r ProviderAccountRepo) SetActive(ctx context.Context, id string, active bool) error {
	return r.update(ctx, map[string]any{"active": active}, "id = ?", id)
}

// ProviderHealthRepo reads and writes provider health snapshots.
type ProviderHealthRepo struct {
	base[domain.ProviderHealthSnapshot]
	db *gorm.DB
}

// ByAccount returns the snapshot for one provider account. A provider that has never
// been probed has none, which is ErrNotFound rather than an unhealthy reading.
func (r ProviderHealthRepo) ByAccount(ctx context.Context, accountID string) (*domain.ProviderHealthSnapshot, error) {
	return r.first(ctx, "provider_account_id = ?", accountID)
}

// Save writes a snapshot, inserting or replacing whichever is needed.
func (r ProviderHealthRepo) Save(ctx context.Context, snap *domain.ProviderHealthSnapshot) error {
	return r.db.WithContext(ctx).Save(snap).Error
}

var providerHealthList = listSpec{
	filters: map[string]filterSpec{
		"status":              {column: "status"},
		"circuit_state":       {column: "circuit_state"},
		"provider_account_id": {column: "provider_account_id"},
	},
	search: []string{"provider_account_id", "last_error_message"},
	sorts:  map[string]string{"latency_ms": "latency_ms", "consecutive_failures": "consecutive_failures", "updated_at": "updated_at"},
	order:  "updated_at desc, provider_account_id desc",
}

// Page returns one page of snapshots, most recently updated first unless q sorts otherwise.
func (r ProviderHealthRepo) Page(ctx context.Context, q ListQuery) (Page[domain.ProviderHealthSnapshot], error) {
	return r.list(ctx, providerHealthList, q)
}

// PaymentRouteRepo reads and writes the routing table.
type PaymentRouteRepo struct{ base[domain.PaymentRoute] }

// UnsupportedMethods returns distinct persisted route methods outside Momobase's enum.
func (r PaymentRouteRepo) UnsupportedMethods(ctx context.Context) ([]string, error) {
	methods := []string{}
	err := r.session(ctx).
		Model(&domain.PaymentRoute{}).
		Distinct("payment_method").
		Where("payment_method NOT IN ?", domain.PaymentMethods()).
		Order("payment_method asc").
		Pluck("payment_method", &methods).Error
	return methods, err
}

// Create stores a new route.
func (r PaymentRouteRepo) Create(ctx context.Context, route *domain.PaymentRoute) error {
	return r.create(ctx, route)
}

// ByID returns one payment route.
func (r PaymentRouteRepo) ByID(ctx context.Context, id string) (*domain.PaymentRoute, error) {
	return r.first(ctx, "id = ?", id)
}

// Candidates returns the active routes for a service in selection order: lowest
// priority first, which is what makes the first match the preferred one. An empty
// service returns every active route, for method discovery.
func (r PaymentRouteRepo) Candidates(ctx context.Context, service string) ([]domain.PaymentRoute, error) {
	const order = "service_type asc, payment_method asc, priority asc"
	if service == "" {
		return r.find(ctx, order, "active = ?", true)
	}
	return r.find(ctx, order, "active = ? AND service_type = ?", true, service)
}

// For returns the active routes that serve one service and payment method, in
// selection order: lowest priority first, then oldest, so the preferred route is the
// first the engine tries and the rest are its fallbacks.
func (r PaymentRouteRepo) For(
	ctx context.Context,
	service string,
	method domain.PaymentMethod,
) ([]domain.PaymentRoute, error) {
	return r.find(ctx, "priority asc, created_at asc",
		"active = ? AND service_type = ? AND payment_method = ?", true, service, method)
}

var paymentRouteList = listSpec{
	filters: map[string]filterSpec{
		"service_type":        {column: "service_type"},
		"payment_method":      {column: "payment_method"},
		"provider_account_id": {column: "provider_account_id"},
		"active":              {column: "active", kind: boolFilter},
	},
	search: []string{"payment_method", "provider_account_id"},
	sorts:  map[string]string{"priority": "priority", "created_at": "created_at"},
	order:  "service_type asc, payment_method asc, priority asc, id asc",
}

// Page returns one page of routes in selection order unless q sorts otherwise.
func (r PaymentRouteRepo) Page(ctx context.Context, q ListQuery) (Page[domain.PaymentRoute], error) {
	return r.list(ctx, paymentRouteList, q)
}

// Update applies the supplied changes to one route.
func (r PaymentRouteRepo) Update(ctx context.Context, id string, values map[string]any) error {
	return r.update(ctx, values, "id = ?", id)
}

// TransactionRepo reads and writes transactions.
type TransactionRepo struct {
	base[domain.Transaction]
	db *gorm.DB
}

// Create stores a new transaction.
func (r TransactionRepo) Create(ctx context.Context, tx *domain.Transaction) error {
	return r.create(ctx, tx)
}

// ByIdempotencyKey returns the transaction an application already created under this
// key, which is what makes a repeat request a replay rather than a second payment.
func (r TransactionRepo) ByIdempotencyKey(ctx context.Context, appID, key string) (*domain.Transaction, error) {
	return r.first(ctx, "app_id = ? AND idempotency_key = ?", appID, key)
}

// ForApp returns one of an application's transactions by a single indexed field, which
// is either its id or its reference. Both are scoped to the application, so one tenant
// can never read another's rows.
func (r TransactionRepo) ForApp(ctx context.Context, appID, field, value string) (*domain.Transaction, error) {
	switch field {
	case "id":
		return r.first(ctx, "app_id = ? AND id = ?", appID, value)
	case "reference":
		return r.first(ctx, "app_id = ? AND reference = ?", appID, value)
	default:
		return nil, ErrNotFound
	}
}

// LockForUpdate re-reads a transaction inside a transaction and holds it until commit.
// A settle path that skipped the lock could apply two provider outcomes at once.
func (r TransactionRepo) LockForUpdate(ctx context.Context, id string) (*domain.Transaction, error) {
	var row domain.Transaction
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&row, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// DueForReconciliation returns up to limit unsettled transactions whose retry time has
// come, oldest first. A transaction with no provider reference is excluded: there is
// nothing to ask the provider about.
func (r TransactionRepo) DueForReconciliation(
	ctx context.Context,
	now time.Time,
	limit int,
) ([]domain.Transaction, error) {
	rows := []domain.Transaction{}
	return rows, r.session(ctx).Where(
		"status IN ? AND provider_reference <> '' AND (next_reconcile_at IS NULL OR next_reconcile_at <= ?)",
		[]string{domain.TxPending, domain.TxProcessing, domain.TxUnknown},
		now,
	).Order("created_at asc").Limit(limit).Find(&rows).Error
}

// ByID returns one transaction across every application, for administrators. App
// callers go through ForApp, which keeps a lookup inside the caller's own tenant.
func (r TransactionRepo) ByID(ctx context.Context, id string) (*domain.Transaction, error) {
	return r.first(ctx, "id = ?", id)
}

// Page returns one page of transactions, newest first unless q sorts otherwise.
func (r TransactionRepo) Page(ctx context.Context, q ListQuery) (Page[domain.Transaction], error) {
	return r.list(ctx, transactionList, q)
}

var transactionList = listSpec{
	filters: map[string]filterSpec{
		"status":                       {column: "status"},
		"service_type":                 {column: "service_type"},
		"payment_method":               {column: "payment_method"},
		"country":                      {column: "country"},
		"currency":                     {column: "currency"},
		"app_id":                       {column: "app_id"},
		"selected_provider_account_id": {column: "selected_provider_account_id"},
	},
	search: []string{"reference", "customer_account", "provider_reference", "id"},
	sorts: map[string]string{
		"created_at": "created_at",
		"amount":     "amount",
		"status":     "status",
		"updated_at": "updated_at",
	},
	order: "created_at desc, id desc",
}

// Update applies the supplied changes to one transaction.
func (r TransactionRepo) Update(ctx context.Context, id string, values map[string]any) error {
	return r.update(ctx, values, "id = ?", id)
}

// UpdateFromStatus applies changes only while the row still holds the status it was
// read at, and reports whether it did. It is how a settle path refuses to overwrite an
// outcome that landed first.
func (r TransactionRepo) UpdateFromStatus(
	ctx context.Context,
	id, previous string,
	values map[string]any,
) (bool, error) {
	result := r.model(ctx).Where("id = ? AND status = ?", id, previous).Updates(values)
	return result.RowsAffected > 0, result.Error
}

// CountReconcileAttempt is the increment a reconciliation pass records, kept here so no
// caller has to name an ORM expression.
func CountReconcileAttempt() any { return gorm.Expr("reconciliation_attempts + 1") }

// TransactionAttemptRepo reads and writes provider attempts against a transaction.
type TransactionAttemptRepo struct {
	base[domain.TransactionAttempt]
}

// Create stores a new attempt.
func (r TransactionAttemptRepo) Create(ctx context.Context, attempt *domain.TransactionAttempt) error {
	return r.create(ctx, attempt)
}

// LatestForTransaction returns a transaction's most recent attempt against one provider
// account, which is the row a settle path has to record its outcome on.
func (r TransactionAttemptRepo) LatestForTransaction(
	ctx context.Context,
	transactionID, accountID string,
) (*domain.TransactionAttempt, error) {
	rows, err := r.find(ctx, "created_at desc",
		"transaction_id = ? AND provider_account_id = ?", transactionID, accountID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	return &rows[0], nil
}

// LatestForReference returns the most recent attempt a provider reference names, which
// is how an inbound webhook or a reconciliation pass finds its transaction.
func (r TransactionAttemptRepo) LatestForReference(
	ctx context.Context,
	accountID, reference string,
) (*domain.TransactionAttempt, error) {
	rows, err := r.find(ctx, "created_at desc",
		"provider_account_id = ? AND provider_reference = ?", accountID, reference)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	return &rows[0], nil
}

// Update applies the supplied changes to one attempt.
func (r TransactionAttemptRepo) Update(ctx context.Context, id string, values map[string]any) error {
	return r.update(ctx, values, "id = ?", id)
}

// WebhookEventRepo reads and writes received provider webhooks.
type WebhookEventRepo struct {
	base[domain.WebhookEvent]
	db *gorm.DB
}

// Insert stores a received webhook and reports whether it was new.
//
// The conflict target is the account and the payload hash, so a provider that delivers
// the same event twice inserts nothing the second time and the caller stops rather than
// applying it again.
func (r WebhookEventRepo) Insert(ctx context.Context, row *domain.WebhookEvent) (bool, error) {
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "provider_account_id"},
			{Name: "payload_hash"},
		},
		DoNothing: true,
	}).Create(row)
	return result.RowsAffected > 0, result.Error
}

// Pending returns up to limit unprocessed events, oldest first.
func (r WebhookEventRepo) Pending(ctx context.Context, limit int) ([]domain.WebhookEvent, error) {
	rows := []domain.WebhookEvent{}
	return rows, r.session(ctx).Where("processed = ?", false).
		Order("created_at asc").Limit(limit).Find(&rows).Error
}

// MarkProcessed records which transaction an event was applied to.
func (r WebhookEventRepo) MarkProcessed(ctx context.Context, id, transactionID string) error {
	return r.update(ctx, map[string]any{
		"transaction_id": transactionID,
		"processed":      true,
	}, "id = ?", id)
}

// DeleteProcessedBefore removes applied events older than cutoff, for the cleanup worker.
func (r WebhookEventRepo) DeleteProcessedBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	result := r.session(ctx).Where("processed = ? AND created_at < ?", true, cutoff).
		Delete(&domain.WebhookEvent{})
	return result.RowsAffected, result.Error
}

// TransactionBucket is one grouped row of the transaction aggregate: a period, the
// dimensions it is grouped by, and the totals for that combination.
type TransactionBucket struct {
	Period      string
	ServiceType string
	Status      string
	Currency    string
	Count       int64
	Amount      int64
}

// AggregateFilter narrows the transaction aggregate. Bucket is the SQL date-truncation
// expression, which is the one part of the query that cannot be written portably.
type AggregateFilter struct {
	Bucket            string
	From              time.Time
	To                time.Time
	AppID             string
	ProviderAccountID string
}

// Aggregate groups transactions in the database rather than in Go, so the result size
// depends on the range asked for rather than on how many rows fall inside it.
func (r TransactionRepo) Aggregate(ctx context.Context, filter AggregateFilter) ([]TransactionBucket, error) {
	rows := []TransactionBucket{}
	query := r.model(ctx).
		Select(filter.Bucket+" AS period, service_type, status, currency, COUNT(*) AS count, SUM(amount) AS amount").
		Where("created_at >= ? AND created_at < ?", filter.From, filter.To).
		Group("period, service_type, status, currency").
		Order("period asc")
	if filter.AppID != "" {
		query = query.Where("app_id = ?", filter.AppID)
	}
	if filter.ProviderAccountID != "" {
		query = query.Where("selected_provider_account_id = ?", filter.ProviderAccountID)
	}
	return rows, query.Find(&rows).Error
}
