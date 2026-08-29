package provider

import (
	"context"
	"errors"
	"time"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/repository"
	"github.com/momobasehq/momobase/providers"
)

// HealthService checks active provider adapters and persists their latest health state.
type HealthService struct {
	repos    *repository.UnitOfWork
	runtime  *RuntimeManager
	executor *Executor
}

// NewHealthService creates a provider health-check service.
func NewHealthService(repos *repository.UnitOfWork, runtime *RuntimeManager) *HealthService {
	return &HealthService{repos, runtime, NewExecutor(runtime)}
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
	// A provider that has never been probed has no snapshot, which is the first-run
	// case rather than a failure.
	stored, err := s.repos.ProviderHealth.ByAccount(ctx, id)
	if err != nil && !repository.IsNotFound(err) {
		return err
	}
	var snap domain.ProviderHealthSnapshot
	if stored != nil {
		snap = *stored
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
		snap.CollectionsAvailable = providers.SupportsService(rp.Capabilities, domain.ServiceCollection)
		snap.DisbursementsAvailable = providers.SupportsService(rp.Capabilities, domain.ServiceDisbursement)
		_, snap.BalanceQueryAvailable = rp.Adapter.(providers.BalanceQuerier)
	}
	return s.repos.ProviderHealth.Save(ctx, &snap)
}
