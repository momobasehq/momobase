package services

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/providers"
)

// HealthService checks active provider adapters and persists their latest health state.
type HealthService struct {
	db       *gorm.DB
	runtime  *ProviderRuntimeManager
	executor *RuntimeProviderExecutor
}

// NewHealthService creates a provider health-check service.
func NewHealthService(db *gorm.DB, runtime *ProviderRuntimeManager) *HealthService {
	return &HealthService{db, runtime, NewProviderExecutor(runtime)}
}

// CheckAll checks every loaded provider, records each result, and joins any failures.
func (s *HealthService) CheckAll(ctx context.Context) error {
	var errs []error
	for _, runtime := range s.runtime.List() {
		start := time.Now()
		err := s.executor.Health(ctx, runtime.AccountID)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if save := s.save(ctx, runtime.AccountID, time.Since(start), err); save != nil {
			err = errors.Join(err, save)
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
func (s *HealthService) save(ctx context.Context, id string, latency time.Duration, cause error) error {
	now := time.Now().UTC()
	var snap domain.ProviderHealthSnapshot
	if err := s.db.WithContext(ctx).First(&snap, "provider_account_id = ?", id).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if snap.ProviderAccountID == "" {
		snap.ProviderAccountID, snap.CreatedAt = id, now
	}
	snap.LastCheckedAt, snap.LatencyMs, snap.UpdatedAt, snap.CircuitState =
		&now,
		int(latency.Milliseconds()),
		now,
		s.runtime.CircuitState(id)
	if cause == nil {
		snap.Status, snap.LastSuccessAt, snap.ConsecutiveFailures, snap.LastErrorCode, snap.LastErrorMessage =
			domain.ProviderHealthy,
			&now,
			0,
			"",
			""
	} else {
		snap.LastFailureAt, snap.ConsecutiveFailures, snap.LastErrorCode, snap.LastErrorMessage =
			&now,
			snap.ConsecutiveFailures+1,
			"health_check_failed",
			providers.Redact(cause.Error())
		snap.Status = domain.ProviderDegraded
		if snap.ConsecutiveFailures >= 3 || snap.CircuitState == domain.CircuitOpen {
			snap.Status = domain.ProviderDown
		}
	}
	if rp, ok := s.runtime.Get(id); ok {
		snap.CollectionsAvailable = providers.Supports(rp.Capabilities, domain.ServiceCollection, domain.PaymentMethodMomo)
		snap.DisbursementsAvailable = providers.Supports(rp.Capabilities, domain.ServiceDisbursement, domain.PaymentMethodMomo)
		snap.BalanceQueryAvailable = snap.CollectionsAvailable || snap.DisbursementsAvailable
	}
	return s.db.WithContext(ctx).Save(&snap).Error
}
