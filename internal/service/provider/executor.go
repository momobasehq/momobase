package provider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/utils"
	"github.com/momobasehq/momobase/providers"
)

// Executor invokes loaded provider adapters with readiness checks, timeouts, logging, and circuit breaking.
type Executor struct {
	runtime *RuntimeManager
	timeout time.Duration
}

// NewExecutor creates a runtime provider executor with the default operation timeout.
func NewExecutor(runtime *RuntimeManager) *Executor {
	return &Executor{runtime, 45 * time.Second}
}

// ValidateRequest lets the selected provider validate and normalize a payment
// request before Momobase persists it. Providers that do not implement
// providers.RequestValidator are skipped.
//
// A rejection is a client error rather than a provider outage, so it deliberately
// bypasses the circuit breaker: a stream of malformed accounts must not take a
// healthy provider out of rotation.
func (e *Executor) ValidateRequest(ctx context.Context, id string, req *providers.PaymentRequest) error {
	p, err := e.ready(id, "", "", req.Country)
	if err != nil {
		return err
	}
	validator, ok := p.Adapter.(providers.RequestValidator)
	if !ok {
		return nil
	}
	before := *req
	c, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	if err = validator.ValidateRequest(c, req); err != nil {
		return fmt.Errorf("provider rejected the payment request: %s", providers.Redact(err.Error()))
	}
	return guardValidatedRequest(before, req)
}

// guardValidatedRequest rejects a provider that rewrote a field it does not own.
// Only the account and its scheme are the provider's to normalize; the rest is
// already hashed for idempotency and about to be persisted as the caller sent it.
func guardValidatedRequest(before providers.PaymentRequest, after *providers.PaymentRequest) error {
	if before.TransactionID != after.TransactionID ||
		before.PaymentMethod != after.PaymentMethod ||
		before.Amount != after.Amount ||
		before.Currency != after.Currency ||
		before.Country != after.Country ||
		before.Reference != after.Reference {
		return errors.New("provider modified a payment request field it does not own")
	}
	if after.Account == "" || !utils.ValidAccount(after.Account) {
		return errors.New("provider normalized the account to an unusable value")
	}
	return nil
}

// Collect executes a collection through a ready provider that supports the request country.
func (e *Executor) Collect(
	ctx context.Context,
	id string,
	req providers.PaymentRequest,
) (*providers.ProviderPaymentResponse, error) {
	p, err := e.ready(id, domain.ServiceCollection, req.PaymentMethod, req.Country)
	if err != nil {
		return nil, err
	}
	return execute(ctx, e, p, "collect", func(c context.Context) (*providers.ProviderPaymentResponse, error) {
		return p.Adapter.(providers.Collector).Collect(c, req)
	})
}

// Disburse executes a disbursement through a ready provider that supports the request country.
func (e *Executor) Disburse(
	ctx context.Context,
	id string,
	req providers.PaymentRequest,
) (*providers.ProviderPaymentResponse, error) {
	p, err := e.ready(id, domain.ServiceDisbursement, req.PaymentMethod, req.Country)
	if err != nil {
		return nil, err
	}
	return execute(ctx, e, p, "disburse", func(c context.Context) (*providers.ProviderPaymentResponse, error) {
		return p.Adapter.(providers.Disburser).Disburse(c, req)
	})
}

// QueryTransaction retrieves a provider transaction's current status.
func (e *Executor) QueryTransaction(
	ctx context.Context,
	id string,
	ref string,
	country string,
) (*providers.ProviderTransactionStatus, error) {
	p, err := e.ready(id, "", "", country)
	if err != nil {
		return nil, err
	}
	querier, ok := p.Adapter.(providers.TransactionQuerier)
	if !ok {
		return nil, providers.ErrOperationUnsupported
	}
	return execute(ctx, e, p, "status", func(c context.Context) (*providers.ProviderTransactionStatus, error) {
		return querier.QueryTransaction(c, ref, country)
	})
}

// QueryBalance retrieves a provider balance for its configured country.
func (e *Executor) QueryBalance(ctx context.Context, id, country string) (*providers.ProviderBalance, error) {
	p, err := e.ready(id, "", "", country)
	if err != nil {
		return nil, err
	}
	querier, ok := p.Adapter.(providers.BalanceQuerier)
	if !ok {
		return nil, providers.ErrOperationUnsupported
	}
	if country == "" {
		country = p.Country
	}
	return execute(ctx, e, p, "balance", func(c context.Context) (*providers.ProviderBalance, error) {
		return querier.QueryBalance(c, country)
	})
}

// Health runs the health check for a loaded provider.
func (e *Executor) Health(ctx context.Context, id string) error {
	p, err := e.ready(id, "", "", "")
	if err != nil {
		return err
	}
	checker, ok := p.Adapter.(providers.HealthChecker)
	if !ok {
		return nil
	}
	_, err = execute(ctx, e, p, "health", func(c context.Context) (struct{}, error) {
		return struct{}{}, checker.HealthCheck(c)
	})
	return err
}

// VerifyWebhook delegates webhook payload verification to a loaded provider within the operation timeout.
func (e *Executor) VerifyWebhook(
	ctx context.Context,
	id string,
	payload []byte,
	headers map[string]string,
) (*providers.ProviderWebhookEvent, error) {
	p, err := e.ready(id, "", "", "")
	if err != nil {
		return nil, err
	}
	verifier, ok := p.Adapter.(providers.WebhookVerifier)
	if !ok {
		return nil, providers.ErrOperationUnsupported
	}
	c, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	return verifier.VerifyWebhook(c, payload, headers)
}

// ready returns a loaded runtime that can serve the requested operation and country.
func (e *Executor) ready(
	id string,
	service string,
	method providers.PaymentMethod,
	country string,
) (*Runtime, error) {
	p, ok := e.runtime.Get(id)
	if !ok || p.Adapter == nil {
		return nil, errors.New("provider not initialized")
	}
	if service != "" && !providers.Supports(p.Capabilities, service, method) {
		return nil, fmt.Errorf("provider does not support %s/%s", service, method)
	}
	if country != "" && p.Country != country {
		return nil, fmt.Errorf("provider does not support country %s", country)
	}
	return p, nil
}

func execute[T any](
	ctx context.Context,
	e *Executor,
	p *Runtime,
	op string,
	call func(context.Context) (T, error),
) (T, error) {
	var zero T
	if err := p.breaker.before(time.Now()); err != nil {
		return zero, err
	}
	c, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	start := time.Now()
	out, err := call(c)
	breakerErr := err
	if ctx.Err() != nil {
		breakerErr = nil // Caller cancellation is not a provider failure.
	}
	p.breaker.after(time.Now(), breakerErr)
	attrs := []any{
		slog.String("provider", p.ProviderCode),
		slog.String("provider_account_id", p.AccountID),
		slog.String("operation", op),
		slog.Int64("duration_ms", time.Since(start).Milliseconds()),
	}
	if err != nil {
		attrs = append(attrs, slog.String("error", providers.Redact(err.Error())))
		e.runtime.logger.Warn("provider operation failed", attrs...)
	} else {
		e.runtime.logger.Info("provider operation completed", attrs...)
	}
	return out, err
}
