package routing

import (
	"context"
	"errors"
	"strings"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/repository"
	"github.com/momobasehq/momobase/internal/service/audit"
	"github.com/momobasehq/momobase/internal/utils"
)

// AdminService manages payment routing rules.
type AdminService struct {
	repos *repository.UnitOfWork
	audit *audit.Service
}

// NewAdminService creates a payment route administration service.
func NewAdminService(repos *repository.UnitOfWork, audit *audit.Service) *AdminService {
	return &AdminService{repos, audit}
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
	if service != domain.ServiceCollection && service != domain.ServiceDisbursement {
		return nil, errors.New("invalid service_type")
	}
	// The payment method is free-form: it names the rail this route serves, which
	// the engine only ever compares against a request, so it is checked for shape
	// rather than against a fixed set.
	method = strings.ToLower(strings.TrimSpace(method))
	if method == "" || !utils.ValidIdentifier(method) {
		return nil, errors.New("payment_method is required and may contain only letters, digits, and _-. and must not exceed 64 characters")
	}
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
	if err := s.repos.PaymentRoutes.Create(ctx, route); err != nil {
		return nil, err
	}
	s.audit.RecordBestEffort(ctx, actor.ActorID(), "admin", "route.created", "payment_route", route.ID, nil, "", "")
	return route, nil
}

// Update replaces a payment route's priority and active state.
func (s *AdminService) Update(ctx context.Context, actor *domain.AdminUser, id string, priority int, active bool) error {
	if priority < 1 {
		return errors.New("priority must be at least 1")
	}
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
