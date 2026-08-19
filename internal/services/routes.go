package services

import (
	"context"
	"errors"
	"slices"
	"strings"

	"gorm.io/gorm"

	"github.com/momobasehq/momobase/internal/audit"
	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/store"
	"github.com/momobasehq/momobase/internal/utils"
	"github.com/momobasehq/momobase/providers"
)

// ErrNoRouteAvailable indicates that no active, healthy provider satisfies a payment route request.
var ErrNoRouteAvailable = errors.New("no active provider route available")

// RouteAdminService manages payment routing rules.
type RouteAdminService struct {
	db    *gorm.DB
	audit *audit.Service
}

// NewRouteAdminService creates a payment route administration service.
func NewRouteAdminService(db *gorm.DB, audit *audit.Service) *RouteAdminService {
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

// SelectProvider returns the highest-priority active provider that supports the
// requested service, method, and country. The country may be empty, which only
// provider accounts that declare no countries of their own can serve.
func (e *RouteEngine) SelectProvider(ctx context.Context, service, method, country string) (*SelectedProvider, error) {
	country, err := utils.NormalizeOptionalCountry(country)
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
		if candidate, ok := e.candidate(ctx, route, service, country); ok {
			return candidate, nil
		}
	}
	return nil, ErrNoRouteAvailable
}

// AvailablePaymentMethod is one method a client may currently pay with.
type AvailablePaymentMethod struct {
	// ServiceType is the service the method is available for.
	ServiceType string `json:"service_type"`
	// PaymentMethod is the value to send as a payment's payment_method.
	PaymentMethod string `json:"payment_method"`
}

// AvailablePaymentMethods lists the methods that would route right now, ordered by
// service then method. Both filters are optional: an empty service covers both, and
// an empty country only matches provider accounts that declare no countries.
//
// Availability is decided by the same candidate check SelectProvider uses, so a
// listed method is one SelectProvider will find. Answering from a second query would
// let the two drift, and the failure mode is offering a payment that then 503s.
//
// Schemes are deliberately absent. Nothing registers them server-side: a scheme is
// free-form text the selected provider interprets, so what values are valid is the
// provider's to document, not Momobase's to enumerate.
func (e *RouteEngine) AvailablePaymentMethods(ctx context.Context, service, country string) ([]AvailablePaymentMethod, error) {
	if service != "" && service != domain.ServiceCollection && service != domain.ServiceDisbursement {
		return nil, errors.New("service_type must be collection or disbursement")
	}
	country, err := utils.NormalizeOptionalCountry(country)
	if err != nil {
		return nil, err
	}
	query := e.db.WithContext(ctx).Where("active = ?", true)
	if service != "" {
		query = query.Where("service_type = ?", service)
	}
	var routes []domain.PaymentRoute
	if err := query.Order("service_type asc, payment_method asc, priority asc").Find(&routes).Error; err != nil {
		return nil, err
	}
	// One method may have several routes; the first that passes makes it available,
	// and the rest are the fallbacks SelectProvider would try.
	seen := make(map[AvailablePaymentMethod]struct{}, len(routes))
	available := make([]AvailablePaymentMethod, 0, len(routes))
	for _, route := range routes {
		method := AvailablePaymentMethod{ServiceType: route.ServiceType, PaymentMethod: route.PaymentMethod}
		if _, done := seen[method]; done {
			continue
		}
		if _, ok := e.candidate(ctx, route, route.ServiceType, country); !ok {
			continue
		}
		seen[method] = struct{}{}
		available = append(available, method)
	}
	return available, nil
}

func (e *RouteEngine) candidate(ctx context.Context, route domain.PaymentRoute, service, country string) (*SelectedProvider, bool) {
	var account domain.ProviderAccount
	if e.db.WithContext(ctx).
		Where("id = ? AND active = ?", route.ProviderAccountID, true).
		First(&account).Error != nil || !countryEligible(account.Countries, country) {
		return nil, false
	}
	rp, ok := e.runtime.Get(account.ID)
	if !ok || !providers.Supports(rp.Capabilities, service) || e.runtime.CircuitState(account.ID) == domain.CircuitOpen {
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

// countryEligible reports whether a provider account may serve a request country.
// An account that declares no countries is unrestricted, which is how a rail with
// no country notion is modelled; an account that declares them requires a request
// country it lists, so there is still no global or fallback country.
func countryEligible(countries []string, country string) bool {
	if len(countries) == 0 {
		return true
	}
	return country != "" && slices.Contains(countries, country)
}
