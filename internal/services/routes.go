package services

import (
	"context"
	"errors"
	"slices"

	"gorm.io/gorm"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/store"
	"github.com/momobasehq/momobase/providers"
)

// ErrNoRouteAvailable indicates that no active, healthy provider satisfies a payment route request.
var ErrNoRouteAvailable = errors.New("no active provider route available")

// RouteAdminService manages payment routing rules.
type RouteAdminService struct {
	db    *gorm.DB
	audit *AuditService
}

// NewRouteAdminService creates a payment route administration service.
func NewRouteAdminService(db *gorm.DB, audit *AuditService) *RouteAdminService {
	return &RouteAdminService{db, audit}
}

// Create validates and persists a payment route for an existing provider account.
func (s *RouteAdminService) Create(
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
	if method != domain.PaymentMethodMomo {
		return nil, errors.New("only payment_method=momo is implemented")
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
	s.audit.RecordBestEffort(ctx, actorID(actor), "admin", "route.created", "payment_route", route.ID, nil, "", "")
	return route, nil
}

// Update replaces a payment route's priority and active state.
func (s *RouteAdminService) Update(ctx context.Context, actor *domain.AdminUser, id string, priority int, active bool) error {
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
		actorID(actor),
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

// SelectedProvider contains the route, account, and loaded runtime chosen for a payment.
type SelectedProvider struct {
	// Route is the payment route that selected the provider account.
	Route domain.PaymentRoute
	// Account is the active persisted provider account.
	Account domain.ProviderAccount
	// Runtime is the account's loaded provider runtime.
	Runtime *RuntimeProvider
}

// RouteEngine selects an eligible provider for a payment request.
type RouteEngine struct {
	db      *gorm.DB
	runtime *ProviderRuntimeManager
}

// NewRouteEngine creates a provider route-selection engine.
func NewRouteEngine(db *gorm.DB, runtime *ProviderRuntimeManager) *RouteEngine {
	return &RouteEngine{db, runtime}
}

// SelectProvider returns the highest-priority active provider that supports the requested service, method, and country.
func (e *RouteEngine) SelectProvider(ctx context.Context, service, method, country string) (*SelectedProvider, error) {
	country, err := NormalizeTransactionCountry(country)
	if err != nil {
		return nil, err
	}
	var routes []domain.PaymentRoute
	if err := e.db.WithContext(ctx).
		Where(
			"active = ? AND service_type = ? AND payment_method = ?",
			true,
			service,
			method,
		).
		Order("priority asc, created_at asc").
		Find(&routes).Error; err != nil {
		return nil, err
	}
	for _, route := range routes {
		if candidate, ok := e.candidate(ctx, route, service, method, country); ok {
			return candidate, nil
		}
	}
	return nil, ErrNoRouteAvailable
}
func (e *RouteEngine) candidate(ctx context.Context, route domain.PaymentRoute, service, method, country string) (*SelectedProvider, bool) {
	var account domain.ProviderAccount
	if e.db.WithContext(ctx).
		Where("id = ? AND active = ?", route.ProviderAccountID, true).
		First(&account).Error != nil || !slices.Contains(account.Countries, country) {
		return nil, false
	}
	rp, ok := e.runtime.Get(account.ID)
	if !ok || !providers.Supports(rp.Capabilities, service, method) || e.runtime.CircuitState(account.ID) == domain.CircuitOpen {
		return nil, false
	}
	var health domain.ProviderHealthSnapshot
	err := e.db.WithContext(ctx).First(&health, "provider_account_id = ?", account.ID).Error
	if err == nil && (health.Status == domain.ProviderDown ||
		health.Status == domain.ProviderDisabled ||
		health.Status == domain.ProviderMisconfigured ||
		health.CircuitState == domain.CircuitOpen) {
		return nil, false
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false
	}
	return &SelectedProvider{route, account, rp}, true
}
