package routing

import (
	"context"
	"errors"

	"github.com/momobasehq/momobase/internal/cache"
	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/repository"
	"github.com/momobasehq/momobase/internal/service/provider"
	"github.com/momobasehq/momobase/internal/utils"
	"github.com/momobasehq/momobase/providers"
)

// ErrNoRouteAvailable indicates that no active, healthy provider satisfies a payment route request.
var ErrNoRouteAvailable = errors.New("no active provider route available")

// SelectedProvider contains the route, account, and loaded runtime chosen for a payment.
type SelectedProvider struct {
	// Route is the payment route that selected the provider account.
	Route domain.PaymentRoute
	// Account is the active persisted provider account.
	Account domain.ProviderAccount
	// Runtime is the account's loaded provider runtime.
	Runtime *provider.Runtime
}

// Engine selects an eligible provider for a payment request.
type Engine struct {
	repos   *repository.UnitOfWork
	runtime *provider.RuntimeManager
	cache   cache.Store
}

// NewEngine creates a provider route-selection engine.
func NewEngine(
	repos *repository.UnitOfWork,
	runtime *provider.RuntimeManager,
	stores ...cache.Store,
) *Engine {
	var store cache.Store
	if len(stores) > 0 {
		store = stores[0]
	}
	return &Engine{repos: repos, runtime: runtime, cache: store}
}

// SelectProvider returns the highest-priority active provider matching the payment's
// service, method, country, and currency.
func (e *Engine) SelectProvider(
	ctx context.Context,
	service string,
	method string,
	country string,
	currency string,
) (*SelectedProvider, error) {
	country, err := utils.NormalizeTransactionCountry(country)
	if err != nil {
		return nil, err
	}
	currency, err = utils.NormalizeCurrency(currency)
	if err != nil {
		return nil, err
	}
	key := routeCacheKey("for", service, method)
	routes := cache.Get[[]domain.PaymentRoute](ctx, e.cache, key)
	if routes == nil {
		loaded, err := e.repos.PaymentRoutes.For(ctx, service, method)
		if err != nil {
			return nil, err
		}
		routes = &loaded
		cache.Set(ctx, e.cache, key, loaded)
	}
	for _, route := range *routes {
		if candidate, ok := e.candidate(ctx, route, service, country, currency); ok {
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
// service then method. An empty service covers both.
//
// Availability is decided by the same candidate check SelectProvider uses, so a
// listed method is one SelectProvider will find. Answering from a second query would
// let the two drift, and the failure mode is offering a payment that then 503s.
//
// Schemes are deliberately absent. Nothing registers them server-side: a scheme is
// free-form text the selected provider interprets, so what values are valid is the
// provider's to document, not Momobase's to enumerate.
func (e *Engine) AvailablePaymentMethods(
	ctx context.Context,
	service string,
	country string,
	currency string,
) ([]AvailablePaymentMethod, error) {
	if service != "" && service != domain.ServiceCollection && service != domain.ServiceDisbursement {
		return nil, errors.New("service_type must be collection or disbursement")
	}
	country, err := utils.NormalizeTransactionCountry(country)
	if err != nil {
		return nil, err
	}
	currency, err = utils.NormalizeCurrency(currency)
	if err != nil {
		return nil, err
	}
	key := routeCacheKey("candidates", service, "")
	routes := cache.Get[[]domain.PaymentRoute](ctx, e.cache, key)
	if routes == nil {
		loaded, err := e.repos.PaymentRoutes.Candidates(ctx, service)
		if err != nil {
			return nil, err
		}
		routes = &loaded
		cache.Set(ctx, e.cache, key, loaded)
	}
	// One method may have several routes; the first that passes makes it available,
	// and the rest are the fallbacks SelectProvider would try.
	seen := make(map[AvailablePaymentMethod]struct{}, len(*routes))
	available := make([]AvailablePaymentMethod, 0, len(*routes))
	for _, route := range *routes {
		method := AvailablePaymentMethod{ServiceType: route.ServiceType, PaymentMethod: route.PaymentMethod}
		if _, done := seen[method]; done {
			continue
		}
		if _, ok := e.candidate(ctx, route, route.ServiceType, country, currency); !ok {
			continue
		}
		seen[method] = struct{}{}
		available = append(available, method)
	}
	return available, nil
}

func routeCacheKey(query, service, method string) string {
	return "routes:v1:" + query + ":" + service + ":" + method
}

func (e *Engine) candidate(
	ctx context.Context,
	route domain.PaymentRoute,
	service string,
	country string,
	currency string,
) (*SelectedProvider, bool) {
	account, err := e.repos.ProviderAccounts.ActiveByID(ctx, route.ProviderAccountID)
	if err != nil || account.Country != country || account.Currency != currency {
		return nil, false
	}
	rp, ok := e.runtime.Get(account.ID)
	if !ok || !providers.Supports(rp.Capabilities, service) || e.runtime.CircuitState(account.ID) == domain.CircuitOpen {
		return nil, false
	}
	health, err := e.repos.ProviderHealth.ByAccount(ctx, account.ID)
	if err == nil && (health.Status == domain.ProviderDown ||
		health.Status == domain.ProviderDisabled ||
		health.Status == domain.ProviderMisconfigured ||
		health.CircuitState == domain.CircuitOpen) {
		return nil, false
	}
	// A provider that has never been probed has no snapshot, which is not a reason to
	// route away from it; any other read failure is.
	if err != nil && !repository.IsNotFound(err) {
		return nil, false
	}
	return &SelectedProvider{route, *account, rp}, true
}
