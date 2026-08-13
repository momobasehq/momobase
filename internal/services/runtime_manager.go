package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/providers"
)

type circuit struct {
	mu       sync.Mutex
	failures int
	opened   time.Time
	probing  bool
}

func (c *circuit) state(now time.Time) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.opened.IsZero() {
		return domain.CircuitClosed
	}
	if now.Sub(c.opened) >= 30*time.Second {
		return domain.CircuitHalfOpen
	}
	return domain.CircuitOpen
}
func (c *circuit) before(now time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.opened.IsZero() {
		return nil
	}
	if now.Sub(c.opened) < 30*time.Second || c.probing {
		return providers.ErrCircuitOpen
	}
	c.probing = true
	return nil
}
func (c *circuit) after(now time.Time, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.probing = false
	if err == nil {
		c.failures, c.opened = 0, time.Time{}
		return
	}
	c.failures++
	if c.failures >= 3 {
		c.opened = now
	}
}

// RuntimeProvider contains an initialized provider adapter and its loaded account configuration.
type RuntimeProvider struct {
	// AccountID is the persisted provider account identifier.
	AccountID string
	// ProviderCode identifies the registered provider adapter implementation.
	ProviderCode string
	// Countries lists the transaction countries supported by the account.
	Countries []string
	// ConfigVersion is the provider configuration version loaded into the adapter.
	ConfigVersion int
	// Adapter is the initialized payment provider implementation.
	Adapter providers.PaymentProvider
	// Capabilities lists the payment operations exposed by the adapter.
	Capabilities []providers.Capability
	// WebhookSecret is the decrypted secret used to authenticate incoming provider webhooks.
	WebhookSecret string
	breaker       *circuit
}

// ProviderRuntimeManager loads provider accounts and exposes their initialized adapters safely across goroutines.
type ProviderRuntimeManager struct {
	mu        sync.RWMutex
	db        *gorm.DB
	registry  providers.Registry
	encryptor *platform.Encryptor
	logger    *slog.Logger
	items     map[string]*RuntimeProvider
}

// NewProviderRuntimeManager creates an empty provider runtime manager.
func NewProviderRuntimeManager(
	db *gorm.DB,
	registry providers.Registry,
	enc *platform.Encryptor,
	log *slog.Logger,
) *ProviderRuntimeManager {
	return &ProviderRuntimeManager{db: db, registry: registry, encryptor: enc, logger: log, items: map[string]*RuntimeProvider{}}
}

// Get returns the loaded runtime for a provider account ID.
func (m *ProviderRuntimeManager) Get(id string) (*RuntimeProvider, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.items[id]
	return p, ok
}

// List returns a snapshot slice containing all loaded provider runtimes.
func (m *ProviderRuntimeManager) List() []*RuntimeProvider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*RuntimeProvider, 0, len(m.items))
	for _, p := range m.items {
		out = append(out, p)
	}
	return out
}

// CircuitState returns a provider's current circuit-breaker state, treating unknown providers as open.
func (m *ProviderRuntimeManager) CircuitState(id string) string {
	if p, ok := m.Get(id); ok {
		return p.breaker.state(time.Now())
	}
	return domain.CircuitOpen
}

// LoadActive initializes every active provider account and joins any account-specific failures.
func (m *ProviderRuntimeManager) LoadActive(ctx context.Context) error {
	var rows []domain.ProviderAccount
	if err := m.db.WithContext(ctx).Where("active = ?", true).Find(&rows).Error; err != nil {
		return err
	}
	var errs []error
	for _, row := range rows {
		if err := m.Reload(ctx, row.ID); err != nil {
			errs = append(errs, fmt.Errorf("reload %s: %w", row.ID, err))
		}
	}
	return errors.Join(errs...)
}

// Disable removes a provider account from the in-memory runtime set.
func (m *ProviderRuntimeManager) Disable(id string) {
	m.mu.Lock()
	delete(m.items, id)
	m.mu.Unlock()
}

// Reload rebuilds an active provider runtime from persisted configuration or removes it when inactive.
func (m *ProviderRuntimeManager) Reload(ctx context.Context, id string) error {
	var account domain.ProviderAccount
	if err := m.db.WithContext(ctx).First(&account, "id = ?", id).Error; err != nil {
		return err
	}
	if !account.Active {
		m.Disable(id)
		return nil
	}
	plain, err := m.plain(&account)
	if err != nil {
		return err
	}
	adapter, err := m.build(ctx, account.ProviderCode, plain)
	if err != nil {
		return err
	}
	caps := adapter.Capabilities()
	fresh := &RuntimeProvider{
		AccountID:     id,
		ProviderCode:  account.ProviderCode,
		Countries:     slices.Clone(account.Countries),
		ConfigVersion: account.ConfigVersion,
		Adapter:       adapter,
		Capabilities:  caps,
		WebhookSecret: providers.String(plain, "webhook_secret"),
		breaker:       &circuit{},
	}
	m.mu.Lock()
	m.items[id] = fresh
	m.mu.Unlock()
	return nil
}

// TestProviderConfig decrypts an account's configuration, initializes its adapter, and runs a health check.
func (m *ProviderRuntimeManager) TestProviderConfig(ctx context.Context, id string) error {
	var account domain.ProviderAccount
	if err := m.db.WithContext(ctx).First(&account, "id = ?", id).Error; err != nil {
		return err
	}
	plain, err := m.plain(&account)
	if err != nil {
		return err
	}
	_, err = m.build(ctx, account.ProviderCode, plain)
	return err
}
func (m *ProviderRuntimeManager) build(
	ctx context.Context,
	code string,
	plain providers.ProviderConfig,
) (providers.PaymentProvider, error) {
	adapter, err := m.registry.Create(code, m.logger.With(slog.String("provider", code)))
	if err != nil {
		return nil, err
	}
	if err = adapter.Init(ctx, plain); err != nil {
		return nil, err
	}
	check, cancel := context.WithTimeout(ctx, 45*time.Second)
	err = adapter.HealthCheck(check)
	cancel()
	if err != nil {
		return nil, err
	}
	return adapter, nil
}
func (m *ProviderRuntimeManager) plain(account *domain.ProviderAccount) (providers.ProviderConfig, error) {
	data, err := m.encryptor.Decrypt(account.EncryptedConfigJSON)
	if err != nil {
		return nil, err
	}
	var plain providers.ProviderConfig
	err = json.Unmarshal(data, &plain)
	return plain, err
}

// QueryBalance retrieves a balance through the specified loaded provider account.
func (m *ProviderRuntimeManager) QueryBalance(ctx context.Context, id, country string) (*providers.ProviderBalance, error) {
	return NewProviderExecutor(m).QueryBalance(ctx, id, country)
}

// ProviderBalanceResult reports the outcome of querying one provider account and country.
type ProviderBalanceResult struct {
	// ProviderAccountID identifies the queried provider account.
	ProviderAccountID string `json:"provider_account_id"`
	// ProviderCode identifies the provider adapter implementation.
	ProviderCode string `json:"provider_code,omitempty"`
	// Country identifies the balance's transaction country.
	Country string `json:"country"`
	// Status is "success" when Balance is populated and "failed" when Error is populated.
	Status string `json:"status"`
	// Balance contains the successful provider response.
	Balance *providers.ProviderBalance `json:"balance,omitempty"`
	// Error contains a redacted provider error for a failed query.
	Error string `json:"error,omitempty"`
}

// QueryActiveBalances queries every loaded provider for each of its configured countries.
func (m *ProviderRuntimeManager) QueryActiveBalances(ctx context.Context) ([]ProviderBalanceResult, error) {
	var out []ProviderBalanceResult
	for _, runtime := range m.List() {
		for _, country := range runtime.Countries {
			item := ProviderBalanceResult{
				ProviderAccountID: runtime.AccountID,
				ProviderCode:      runtime.ProviderCode,
				Country:           country,
				Status:            "failed",
			}
			balance, err := m.QueryBalance(ctx, runtime.AccountID, country)
			if err != nil {
				item.Error = providers.Redact(err.Error())
			} else {
				item.Status, item.Balance = "success", balance
			}
			out = append(out, item)
		}
	}
	return out, nil
}
