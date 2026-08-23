package payment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/momobasehq/momobase/internal/cache"
	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/dto"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/repository"
	"github.com/momobasehq/momobase/internal/service/provider"
	"github.com/momobasehq/momobase/internal/service/routing"
	"github.com/momobasehq/momobase/internal/utils"
	"github.com/momobasehq/momobase/providers"
)

// CreatePaymentResponse describes the transaction created for a payment request.
type CreatePaymentResponse struct {
	// TransactionID is Momobase's unique transaction identifier.
	TransactionID string `json:"transaction_id"`
	// Reference is the application's business reference.
	Reference string `json:"reference"`
	// ServiceType identifies whether the transaction is a collection or disbursement.
	ServiceType string `json:"service_type"`
	// PaymentMethod identifies the payment rail used for the transaction.
	PaymentMethod string `json:"payment_method"`
	// Status is the current normalized transaction status.
	Status string `json:"status"`
	// SelectedProvider is the provider code selected by the route engine.
	SelectedProvider string `json:"selected_provider"`
	// ProviderReference is the provider-assigned transaction reference, when available.
	ProviderReference string `json:"provider_reference"`
	// Message contains a human-readable result or replay description.
	Message string `json:"message"`
}

// Orchestrator validates, routes, executes, and persists payment requests.
type Orchestrator struct {
	repos    *repository.UnitOfWork
	router   *routing.Engine
	executor *provider.Executor
	cache    cache.Store
}

// NewOrchestrator creates a payment orchestrator.
func NewOrchestrator(
	repos *repository.UnitOfWork,
	router *routing.Engine,
	executor *provider.Executor,
	stores ...cache.Store,
) *Orchestrator {
	var store cache.Store
	if len(stores) > 0 {
		store = stores[0]
	}
	return &Orchestrator{repos: repos, router: router, executor: executor, cache: store}
}

// Get returns one transaction belonging to appID. Only terminal transactions are
// cached because all other states may change while clients are polling them.
func (o *Orchestrator) Get(
	ctx context.Context,
	appID string,
	field string,
	value string,
) (*domain.Transaction, error) {
	key := transactionCacheKey(appID, field, value)
	if tx := cache.Get[domain.Transaction](ctx, o.cache, key); tx != nil {
		return tx, nil
	}
	tx, err := o.repos.Transactions.ForApp(ctx, appID, field, value)
	if err != nil {
		return nil, err
	}
	if domain.Terminal(tx.Status) {
		cache.Set(ctx, o.cache, transactionCacheKey(appID, "id", tx.ID), tx)
		cache.Set(ctx, o.cache, transactionCacheKey(appID, "reference", tx.Reference), tx)
	}
	return tx, nil
}

func transactionCacheKey(appID, field, value string) string {
	return "transaction:v1:app:" + appID + ":" + field + ":" + value
}

// Create idempotently creates, routes, executes, and records a collection or disbursement.
func (o *Orchestrator) Create(
	ctx context.Context,
	appID string,
	service string,
	key string,
	req *dto.CreatePayment,
) (*CreatePaymentResponse, error) {
	// The payload validates itself, and normalizes before it does: the hash below is
	// taken over the normalized request, so what counts as a replay is decided here.
	if err := dto.Check(req); err != nil {
		return nil, err
	}
	if strings.TrimSpace(key) == "" {
		return nil, errors.New("Idempotency-Key header is required")
	}
	hash := paymentRequestHash(service, req)
	if tx, err := o.find(ctx, appID, key); err == nil {
		return replay(tx, hash, service, req)
	} else if !repository.IsNotFound(err) {
		return nil, err
	}
	selected, err := o.router.SelectProvider(ctx, service, req.PaymentMethod, req.Country)
	if err != nil {
		return nil, err
	}
	var name, email string
	if party := req.Party(service); party != nil {
		name, email = party.Name, party.Email
	}
	// The provider request is assembled before the transaction row so that the
	// selected provider can validate and normalize the account first: a rejection
	// must leave no transaction behind, and the account it normalizes to is what
	// gets persisted and what webhook matching later compares against.
	call := providers.PaymentRequest{
		TransactionID: platform.NewID("txn"),
		PaymentMethod: req.PaymentMethod,
		Currency:      req.Currency,
		Country:       req.Country,
		Reference:     req.Reference,
		Account:       req.Account,
		Scheme:        req.Scheme,
		Metadata:      req.Metadata,
		Name:          name,
		Email:         email,
		Description:   req.Description,
		Amount:        req.Amount,
	}
	if err = o.executor.ValidateRequest(ctx, selected.Account.ID, &call); err != nil {
		return nil, err
	}
	tx := &domain.Transaction{
		BaseModel:                 domain.BaseModel{ID: call.TransactionID},
		AppID:                     appID,
		ServiceType:               service,
		PaymentMethod:             req.PaymentMethod,
		Amount:                    req.Amount,
		Currency:                  req.Currency,
		Country:                   req.Country,
		Reference:                 req.Reference,
		IdempotencyKey:            key,
		Status:                    domain.TxProcessing,
		SelectedRouteID:           selected.Route.ID,
		SelectedProviderAccountID: selected.Account.ID,
		CustomerAccount:           call.Account,
		CustomerEmail:             email,
		CustomerName:              name,
		Description:               req.Description,
		RequestHash:               hash,
	}
	attempt := &domain.TransactionAttempt{
		BaseModel:         domain.BaseModel{ID: platform.NewID("att")},
		TransactionID:     tx.ID,
		ProviderAccountID: selected.Account.ID,
		ProviderCode:      selected.Runtime.ProviderCode,
		Status:            domain.TxProcessing,
		RequestHash:       hash,
		StartedAt:         time.Now().UTC(),
	}
	// The transaction and its first attempt are written together: an attempt with no
	// transaction, or a transaction with no attempt, is a row nothing can settle.
	if err := o.repos.Within(ctx, func(r *repository.Set) error {
		if err := r.Transactions.Create(ctx, tx); err != nil {
			return err
		}
		return r.TransactionAttempts.Create(ctx, attempt)
	}); err != nil {
		if existing, findErr := o.find(ctx, appID, key); findErr == nil {
			return replay(existing, hash, service, req)
		}
		return nil, err
	}
	var result *providers.ProviderPaymentResponse
	if service == domain.ServiceCollection {
		result, err = o.executor.Collect(ctx, selected.Account.ID, call)
	} else {
		result, err = o.executor.Disburse(ctx, selected.Account.ID, call)
	}
	return o.persist(ctx, tx, attempt, selected.Runtime.ProviderCode, result, err)
}

func (o *Orchestrator) persist(
	ctx context.Context,
	tx *domain.Transaction,
	attempt *domain.TransactionAttempt,
	providerCode string,
	result *providers.ProviderPaymentResponse,
	cause error,
) (*CreatePaymentResponse, error) {
	now := time.Now().UTC()
	attemptUpdates := map[string]any{}
	txUpdates := map[string]any{"next_reconcile_at": nil}
	message := "provider response unknown"
	if cause != nil || result == nil {
		if cause == nil {
			cause = errors.New("provider returned no response")
		}
		if err := tx.Transition(domain.TxUnknown); err != nil {
			return nil, err
		}
		attemptUpdates["status"], txUpdates["status"] = tx.Status, tx.Status
		attemptUpdates["error_code"], attemptUpdates["error_message"] = "PROVIDER_ERROR", providers.Redact(cause.Error())
	} else {
		target := providers.PaymentStatus(result.Status)
		if err := tx.Transition(target); err != nil {
			return nil, err
		}
		tx.ProviderReference, message = result.ProviderReference, result.Message
		raw, _ := json.Marshal(utils.RedactRawMap(result.Raw))
		attemptUpdates["status"], attemptUpdates["provider_reference"], attemptUpdates["raw_response"] =
			tx.Status,
			tx.ProviderReference,
			string(raw)
		txUpdates["status"], txUpdates["provider_reference"] = tx.Status, tx.ProviderReference
	}
	if domain.Terminal(tx.Status) {
		attemptUpdates["completed_at"] = &now
	} else {
		next := now.Add(time.Minute)
		txUpdates["next_reconcile_at"] = &next
	}
	if err := o.repos.Within(ctx, func(r *repository.Set) error {
		if err := r.TransactionAttempts.Update(ctx, attempt.ID, attemptUpdates); err != nil {
			return err
		}
		return r.Transactions.Update(ctx, tx.ID, txUpdates)
	}); err != nil {
		return nil, fmt.Errorf("provider result persistence failed: %w", err)
	}
	return &CreatePaymentResponse{
		TransactionID:     tx.ID,
		Reference:         tx.Reference,
		ServiceType:       tx.ServiceType,
		PaymentMethod:     tx.PaymentMethod,
		Status:            tx.Status,
		SelectedProvider:  providerCode,
		ProviderReference: tx.ProviderReference,
		Message:           message,
	}, nil
}

func (o *Orchestrator) find(ctx context.Context, appID, key string) (*domain.Transaction, error) {
	return o.repos.Transactions.ByIdempotencyKey(ctx, appID, key)
}

func replay(tx *domain.Transaction, hash, _ string, _ *dto.CreatePayment) (*CreatePaymentResponse, error) {
	if tx.RequestHash != hash {
		return nil, errors.New("idempotency key reused with a different request")
	}
	return &CreatePaymentResponse{
		TransactionID:     tx.ID,
		Reference:         tx.Reference,
		ServiceType:       tx.ServiceType,
		PaymentMethod:     tx.PaymentMethod,
		Status:            tx.Status,
		ProviderReference: tx.ProviderReference,
		Message:           "idempotent replay",
	}, nil
}
