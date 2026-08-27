package routing

import (
	"context"

	"github.com/momobasehq/momobase/internal/cache"
	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/repository"
	"github.com/momobasehq/momobase/internal/service/audit"
)

// AdminService manages payment routing rules.
type AdminService struct {
	repos *repository.UnitOfWork
	audit *audit.Service
	cache *cache.RedisStore
}

// NewAdminService creates a payment route administration service.
func NewAdminService(
	repos *repository.UnitOfWork,
	audit *audit.Service,
	store *cache.RedisStore,
) *AdminService {
	return &AdminService{repos: repos, audit: audit, cache: store}
}

// Create validates and persists a payment route for an existing provider account.
func (s *AdminService) Create(
	ctx context.Context,
	actor *domain.AdminUser,
	service string,
	method string,
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
	methodRoutes := []domain.PaymentRoute{}
	serviceRoutes := []domain.PaymentRoute{}
	allRoutes := []domain.PaymentRoute{}
	if err := s.repos.Within(ctx, func(r *repository.Set) error {
		if err := r.PaymentRoutes.Create(ctx, route); err != nil {
			return err
		}
		var err error
		methodRoutes, err = r.PaymentRoutes.For(ctx, service, method)
		if err != nil {
			return err
		}
		serviceRoutes, err = r.PaymentRoutes.Candidates(ctx, service)
		if err != nil {
			return err
		}
		allRoutes, err = r.PaymentRoutes.Candidates(ctx, "")
		return err
	}); err != nil {
		return nil, err
	}
	s.audit.RecordBestEffort(ctx, actor.ActorID(), "admin", "route.created", "payment_route", route.ID, nil, "", "")
	cache.Set(ctx, s.cache, routeCacheKey("for", service, method), methodRoutes)
	cache.Set(ctx, s.cache, routeCacheKey("candidates", service, ""), serviceRoutes)
	cache.Set(ctx, s.cache, routeCacheKey("candidates", "", ""), allRoutes)
	return route, nil
}

// Update replaces a payment route's priority and active state.
func (s *AdminService) Update(ctx context.Context, actor *domain.AdminUser, id string, priority int, active bool) error {
	var route *domain.PaymentRoute
	methodRoutes := []domain.PaymentRoute{}
	serviceRoutes := []domain.PaymentRoute{}
	allRoutes := []domain.PaymentRoute{}
	if err := s.repos.Within(ctx, func(r *repository.Set) error {
		if err := r.PaymentRoutes.Update(ctx, id, map[string]any{
			"priority": priority,
			"active":   active,
		}); err != nil {
			return err
		}
		var err error
		route, err = r.PaymentRoutes.ByID(ctx, id)
		if err != nil {
			return err
		}
		methodRoutes, err = r.PaymentRoutes.For(ctx, route.ServiceType, route.PaymentMethod)
		if err != nil {
			return err
		}
		serviceRoutes, err = r.PaymentRoutes.Candidates(ctx, route.ServiceType)
		if err != nil {
			return err
		}
		allRoutes, err = r.PaymentRoutes.Candidates(ctx, "")
		return err
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
	cache.Set(ctx, s.cache, routeCacheKey("for", route.ServiceType, route.PaymentMethod), methodRoutes)
	cache.Set(ctx, s.cache, routeCacheKey("candidates", route.ServiceType, ""), serviceRoutes)
	cache.Set(ctx, s.cache, routeCacheKey("candidates", "", ""), allRoutes)
	return nil
}
