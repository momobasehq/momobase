package routing

import (
	"context"
	"errors"
	"fmt"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/repository"
	"github.com/momobasehq/momobase/internal/service/audit"
	"github.com/momobasehq/momobase/internal/service/provider"
	"github.com/momobasehq/momobase/providers"
)

// AdminService manages payment routing rules.
type AdminService struct {
	repos   *repository.UnitOfWork
	audit   *audit.Service
	runtime *provider.RuntimeManager
}

// NewAdminService creates a payment route administration service.
func NewAdminService(
	repos *repository.UnitOfWork,
	audit *audit.Service,
	runtime *provider.RuntimeManager,
) *AdminService {
	return &AdminService{repos: repos, audit: audit, runtime: runtime}
}

// Create validates and persists a payment route for an existing provider account.
func (s *AdminService) Create(
	ctx context.Context,
	actor *domain.AdminUser,
	service string,
	method domain.PaymentMethod,
	accountID string,
	priority int,
	active bool,
) (*domain.PaymentRoute, error) {
	// A route pointing at an account that does not exist would select nothing at
	// payment time, which is a failure the operator should see now instead.
	exists, err := s.repos.ProviderAccounts.Exists(ctx, accountID)
	if err != nil || !exists {
		return nil, repository.ErrNotFound
	}
	loaded, ok := s.runtime.Get(accountID)
	if !ok {
		return nil, errors.New("provider account must be active before creating a route")
	}
	if !providers.Supports(loaded.Capabilities, service, method) {
		return nil, fmt.Errorf("provider account does not support %s/%s", service, method)
	}
	if priority < 1 {
		priority = 1
	}
	route := &domain.PaymentRoute{
		BaseModel:         domain.BaseModel{ID: platform.NewID("route")},
		ServiceType:       service,
		PaymentMethod:     method,
		ProviderAccountID: accountID,
		Priority:          priority,
		Active:            active,
	}
	if err := s.repos.PaymentRoutes.Create(ctx, route); err != nil {
		return nil, err
	}
	s.audit.RecordBestEffort(ctx, actor.ActorID(), "admin", "route.created", "payment_route", route.ID, nil, "", "")
	return route, nil
}

// Update replaces a payment route's priority and active state.
func (s *AdminService) Update(ctx context.Context, actor *domain.AdminUser, id string, priority int, active bool) error {
	if err := s.repos.PaymentRoutes.Update(ctx, id, map[string]any{
		"priority": priority,
		"active":   active,
	}); err != nil {
		return err
	}
	s.audit.RecordBestEffort(
		ctx,
		actor.ActorID(),
		"admin",
		"route.updated",
		"payment_route",
		id,
		map[string]any{"priority": priority, "active": active},
		"",
		"",
	)
	return nil
}
