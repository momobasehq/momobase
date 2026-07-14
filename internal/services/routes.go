package services

import (
	"context"
	"errors"
	"slices"

	"gorm.io/gorm"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/providers"
	"github.com/momobasehq/momobase/internal/store"
)

var ErrNoRouteAvailable = errors.New("no active provider route available")

type RouteAdminService struct {
	db    *gorm.DB
	audit *AuditService
}

func NewRouteAdminService(db *gorm.DB, audit *AuditService) *RouteAdminService {
	return &RouteAdminService{db, audit}
}
func (s *RouteAdminService) Create(actor *domain.AdminUser, service, method, accountID string, priority int, active bool) (*domain.PaymentRoute, error) {
	if service != domain.ServiceCollection && service != domain.ServiceDisbursement {
		return nil, errors.New("invalid service_type")
	}
	if method != domain.PaymentMethodMomo {
		return nil, errors.New("only payment_method=momo is implemented")
	}
	var count int64
	if err := s.db.Model(&domain.ProviderAccount{}).Where("id = ?", accountID).Count(&count).Error; err != nil || count != 1 {
		return nil, gorm.ErrRecordNotFound
	}
	if priority < 1 {
		priority = 1
	}
	route := &domain.PaymentRoute{BaseModel: domain.BaseModel{ID: platform.NewID("route")}, ServiceType: service, PaymentMethod: method, ProviderAccountID: accountID, Priority: priority, Active: active}
	if err := s.db.Create(route).Error; err != nil {
		return nil, err
	}
	s.audit.RecordBestEffort(actorID(actor), "admin", "route.created", "payment_route", route.ID, nil, "", "")
	return route, nil
}
func (s *RouteAdminService) Update(actor *domain.AdminUser, id string, priority int, active bool) error {
	if priority < 1 {
		return errors.New("priority must be at least 1")
	}
	if err := store.Affected(s.db.Model(&domain.PaymentRoute{}).Where("id = ?", id).Updates(map[string]any{"priority": priority, "active": active})); err != nil {
		return err
	}
	s.audit.RecordBestEffort(actorID(actor), "admin", "route.updated", "payment_route", id, map[string]any{"priority": priority, "active": active}, "", "")
	return nil
}

type SelectedProvider struct {
	Route   domain.PaymentRoute
	Account domain.ProviderAccount
	Runtime *RuntimeProvider
}
type RouteEngine struct {
	db      *gorm.DB
	runtime *ProviderRuntimeManager
}

func NewRouteEngine(db *gorm.DB, runtime *ProviderRuntimeManager) *RouteEngine {
	return &RouteEngine{db, runtime}
}
func (e *RouteEngine) SelectProvider(ctx context.Context, service, method, country string) (*SelectedProvider, error) {
	country, err := NormalizeTransactionCountry(country)
	if err != nil {
		return nil, err
	}
	var routes []domain.PaymentRoute
	if err := e.db.WithContext(ctx).Where("active = ? AND service_type = ? AND payment_method = ?", true, service, method).Order("priority asc, created_at asc").Find(&routes).Error; err != nil {
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
	if e.db.WithContext(ctx).Where("id = ? AND active = ?", route.ProviderAccountID, true).First(&account).Error != nil || !slices.Contains(account.Countries, country) {
		return nil, false
	}
	rp, ok := e.runtime.Get(account.ID)
	if !ok || !providers.Supports(rp.Capabilities, service, method) || e.runtime.CircuitState(account.ID) == domain.CircuitOpen {
		return nil, false
	}
	var health domain.ProviderHealthSnapshot
	err := e.db.WithContext(ctx).First(&health, "provider_account_id = ?", account.ID).Error
	if err == nil && (health.Status == domain.ProviderDown || health.Status == domain.ProviderDisabled || health.Status == domain.ProviderMisconfigured || health.CircuitState == domain.CircuitOpen) {
		return nil, false
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false
	}
	return &SelectedProvider{route, account, rp}, true
}
