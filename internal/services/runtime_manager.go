package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"gorm.io/gorm"

	"momobase/internal/domain"
	"momobase/internal/platform"
	"momobase/internal/providers"
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

type RuntimeProvider struct {
	AccountID, ProviderCode string
	ConfigVersion           int
	Adapter                 providers.PaymentProvider
	Capabilities            []providers.Capability
	WebhookSecret           string
	breaker                 *circuit
}
type ProviderRuntimeManager struct {
	mu        sync.RWMutex
	db        *gorm.DB
	registry  providers.Registry
	encryptor *platform.Encryptor
	logger    *slog.Logger
	items     map[string]*RuntimeProvider
}

func NewProviderRuntimeManager(db *gorm.DB, registry providers.Registry, enc *platform.Encryptor, log *slog.Logger) *ProviderRuntimeManager {
	return &ProviderRuntimeManager{db: db, registry: registry, encryptor: enc, logger: log, items: map[string]*RuntimeProvider{}}
}
func (m *ProviderRuntimeManager) Get(id string) (*RuntimeProvider, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.items[id]
	return p, ok
}
func (m *ProviderRuntimeManager) List() []*RuntimeProvider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*RuntimeProvider, 0, len(m.items))
	for _, p := range m.items {
		out = append(out, p)
	}
	return out
}
func (m *ProviderRuntimeManager) CircuitState(id string) string {
	if p, ok := m.Get(id); ok {
		return p.breaker.state(time.Now())
	}
	return domain.CircuitOpen
}
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
func (m *ProviderRuntimeManager) Disable(id string) {
	m.mu.Lock()
	delete(m.items, id)
	m.mu.Unlock()
}
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
	fresh := &RuntimeProvider{AccountID: id, ProviderCode: account.ProviderCode, ConfigVersion: account.ConfigVersion, Adapter: adapter, Capabilities: caps, WebhookSecret: providers.String(plain, "webhook_secret"), breaker: &circuit{}}
	m.mu.Lock()
	m.items[id] = fresh
	m.mu.Unlock()
	return nil
}
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
func (m *ProviderRuntimeManager) build(ctx context.Context, code string, plain providers.ProviderConfig) (providers.PaymentProvider, error) {
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
func (m *ProviderRuntimeManager) QueryBalance(ctx context.Context, id string) (*providers.ProviderBalance, error) {
	return NewProviderExecutor(m).QueryBalance(ctx, id)
}

type ProviderBalanceResult struct {
	ProviderAccountID string                     `json:"provider_account_id"`
	ProviderCode      string                     `json:"provider_code,omitempty"`
	Status            string                     `json:"status"`
	Balance           *providers.ProviderBalance `json:"balance,omitempty"`
	Error             string                     `json:"error,omitempty"`
}

func (m *ProviderRuntimeManager) QueryActiveBalances(ctx context.Context) ([]ProviderBalanceResult, error) {
	runtimes := m.List()
	out := make([]ProviderBalanceResult, 0, len(runtimes))
	for _, runtime := range runtimes {
		item := ProviderBalanceResult{ProviderAccountID: runtime.AccountID, ProviderCode: runtime.ProviderCode, Status: "failed"}
		balance, err := m.QueryBalance(ctx, runtime.AccountID)
		if err != nil {
			item.Error = providers.Redact(err.Error())
		} else {
			item.Status, item.Balance = "success", balance
		}
		out = append(out, item)
	}
	return out, nil
}
