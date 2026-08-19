package routing

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/momobasehq/momobase/internal/audit"
	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/store"
	"github.com/momobasehq/momobase/internal/utils"
)

// AdminService manages payment routing rules.
type AdminService struct {
	db    *gorm.DB
	audit *audit.Service
}

// NewAdminService creates a payment route administration service.
func NewAdminService(db *gorm.DB, audit *audit.Service) *AdminService {
	return &AdminService{db, audit}
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
	var count int64
	db := s.db.WithContext(ctx)
	if err := db.Model(&domain.ProviderAccount{}).Where("id = ?", accountID).Count(&count).Error; err != nil || count != 1 {
		return nil, gorm.ErrRecordNotFound
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
	if err := db.Create(route).Error; err != nil {
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
	if err := store.Affected(
		s.db.WithContext(ctx).Model(&domain.PaymentRoute{}).
			Where("id = ?", id).
			Updates(map[string]any{"priority": priority, "active": active}),
	); err != nil {
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
