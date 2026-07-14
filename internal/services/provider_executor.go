package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/providers"
)

type RuntimeProviderExecutor struct {
	runtime *ProviderRuntimeManager
	timeout time.Duration
}

func NewProviderExecutor(runtime *ProviderRuntimeManager) *RuntimeProviderExecutor {
	return &RuntimeProviderExecutor{runtime, 45 * time.Second}
}

func (e *RuntimeProviderExecutor) Collect(ctx context.Context, id string, req providers.PaymentRequest) (*providers.ProviderPaymentResponse, error) {
	p, err := e.ready(id, domain.ServiceCollection, req.Country)
	if err != nil {
		return nil, err
	}
	return execute(ctx, e, p, "collect", func(c context.Context) (*providers.ProviderPaymentResponse, error) { return p.Adapter.Collect(c, req) })
}

func (e *RuntimeProviderExecutor) Disburse(ctx context.Context, id string, req providers.PaymentRequest) (*providers.ProviderPaymentResponse, error) {
	p, err := e.ready(id, domain.ServiceDisbursement, req.Country)
	if err != nil {
		return nil, err
	}
	return execute(ctx, e, p, "disburse", func(c context.Context) (*providers.ProviderPaymentResponse, error) { return p.Adapter.Disburse(c, req) })
}

func (e *RuntimeProviderExecutor) QueryTransaction(ctx context.Context, id, ref, country string) (*providers.ProviderTransactionStatus, error) {
	p, err := e.ready(id, "", country)
	if err != nil {
		return nil, err
	}
	return execute(ctx, e, p, "status", func(c context.Context) (*providers.ProviderTransactionStatus, error) {
		return p.Adapter.QueryTransaction(c, ref, country)
	})
}

func (e *RuntimeProviderExecutor) QueryBalance(ctx context.Context, id, country string) (*providers.ProviderBalance, error) {
	p, err := e.ready(id, "", country)
	if err != nil {
		return nil, err
	}
	if country == "" {
		if len(p.Countries) != 1 {
			return nil, errors.New("country is required for a multi-country provider")
		}
		country = p.Countries[0]
	}
	return execute(ctx, e, p, "balance", func(c context.Context) (*providers.ProviderBalance, error) { return p.Adapter.QueryBalance(c, country) })
}

func (e *RuntimeProviderExecutor) Health(ctx context.Context, id string) error {
	p, err := e.ready(id, "", "")
	if err != nil {
		return err
	}
	_, err = execute(ctx, e, p, "health", func(c context.Context) (struct{}, error) { return struct{}{}, p.Adapter.HealthCheck(c) })
	return err
}

func (e *RuntimeProviderExecutor) VerifyWebhook(ctx context.Context, id string, payload []byte, headers map[string]string) (*providers.ProviderWebhookEvent, error) {
	p, err := e.ready(id, "", "")
	if err != nil {
		return nil, err
	}
	c, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	return p.Adapter.VerifyWebhook(c, payload, headers)
}

func (e *RuntimeProviderExecutor) ready(id, service, country string) (*RuntimeProvider, error) {
	p, ok := e.runtime.Get(id)
	if !ok || p.Adapter == nil {
		return nil, errors.New("provider not initialized")
	}
	if service != "" && !providers.Supports(p.Capabilities, service, domain.PaymentMethodMomo) {
		return nil, fmt.Errorf("provider does not support %s/momo", service)
	}
	if country != "" && !slices.Contains(p.Countries, country) {
		return nil, fmt.Errorf("provider does not support country %s", country)
	}
	if len(p.Countries) == 0 {
		return nil, errors.New("provider has no supported countries")
	}
	return p, nil
}

func execute[T any](ctx context.Context, e *RuntimeProviderExecutor, p *RuntimeProvider, op string, call func(context.Context) (T, error)) (T, error) {
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
	attrs := []any{slog.String("provider", p.ProviderCode), slog.String("provider_account_id", p.AccountID), slog.String("operation", op), slog.Int64("duration_ms", time.Since(start).Milliseconds())}
	if err != nil {
		attrs = append(attrs, slog.String("error", providers.Redact(err.Error())))
		e.runtime.logger.Warn("provider operation failed", attrs...)
	} else {
		e.runtime.logger.Info("provider operation completed", attrs...)
	}
	return out, err
}
