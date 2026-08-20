package payment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/provider"
	"github.com/momobasehq/momobase/internal/routing"
	"github.com/momobasehq/momobase/internal/store"
	"github.com/momobasehq/momobase/internal/utils"
	"github.com/momobasehq/momobase/providers"
)

// PartyPayload contains the identifying details of a payment party.
type PartyPayload struct {
	// Name is the party's display name.
	Name string `json:"name"`
	// Email is the party's email address.
	Email string `json:"email"`
}

// CreatePaymentRequest contains the common fields used to initiate a collection or disbursement.
type CreatePaymentRequest struct {
	// PaymentMethod identifies the requested payment rail. It is free-form and must
	// match an active payment route.
	PaymentMethod string `json:"payment_method"`
	// Amount is the payment amount in the currency's minor unit.
	Amount int64 `json:"amount"`
	// Currency is the three-letter currency code.
	Currency string `json:"currency"`
	// Country is the optional ISO 3166-1 alpha-2 transaction country. Providers that
	// declare supported countries are only eligible when it is present and matches.
	Country string `json:"country,omitempty"`
	// Reference is the application's unique business reference.
	Reference string `json:"reference"`
	// Description is optional payment context shown to downstream systems.
	Description string `json:"description"`
	// Account is the provider-specific account the payment is collected from or
	// disbursed to: a mobile number, bank account, card token, or wallet address.
	// Momobase treats it as opaque and leaves its validation to the selected
	// provider, through providers.RequestValidator.
	Account string `json:"account"`
	// Scheme optionally names the account's provider-specific scheme, such as a
	// mobile network, bank, or card brand. Like PaymentMethod it is free-form; the
	// selected provider interprets it and Momobase never matches on it.
	Scheme string `json:"scheme,omitempty"`
	// Metadata optionally carries provider-specific payment details, such as a bank
	// branch code. It reaches the selected provider and is never persisted, so it
	// cannot become a free-form store of identifiers Momobase would then have to
	// protect. It is part of the idempotency hash.
	Metadata map[string]any `json:"metadata,omitempty"`
	// Customer optionally identifies the collection customer.
	Customer *PartyPayload `json:"customer,omitempty"`
	// Recipient optionally identifies the disbursement recipient.
	Recipient *PartyPayload `json:"recipient,omitempty"`
}

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
	db       *gorm.DB
	router   *routing.Engine
	executor *provider.Executor
}

// NewOrchestrator creates a payment orchestrator.
func NewOrchestrator(db *gorm.DB, router *routing.Engine, executor *provider.Executor) *Orchestrator {
	return &Orchestrator{db, router, executor}
}

// Create idempotently creates, routes, executes, and records a collection or disbursement.
func (o *Orchestrator) Create(
	ctx context.Context,
	appID string,
	service string,
	key string,
	req *CreatePaymentRequest,
) (*CreatePaymentResponse, error) {
	if err := ValidatePaymentPayload(service, req); err != nil {
		return nil, err
	}
	if strings.TrimSpace(key) == "" {
		return nil, errors.New("Idempotency-Key header is required")
	}
	hash := paymentRequestHash(service, req)
	if tx, err := o.find(ctx, appID, key); err == nil {
		return replay(tx, hash, service, req)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	selected, err := o.router.SelectProvider(ctx, service, req.PaymentMethod, req.Country)
	if err != nil {
		return nil, err
	}
	var name, email string
	if party := paymentParty(service, req); party != nil {
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
	if err := store.Within(ctx, o.db, func(db *gorm.DB) error {
		if err := db.Create(tx).Error; err != nil {
			return err
		}
		return db.Create(attempt).Error
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
	if err := store.Within(ctx, o.db, func(db *gorm.DB) error {
		if err := store.Affected(db.Model(&domain.TransactionAttempt{}).Where("id = ?", attempt.ID).Updates(attemptUpdates)); err != nil {
			return err
		}
		return store.Affected(db.Model(&domain.Transaction{}).Where("id = ?", tx.ID).Updates(txUpdates))
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
	var tx domain.Transaction
	return &tx, o.db.WithContext(ctx).Where("app_id = ? AND idempotency_key = ?", appID, key).First(&tx).Error
}

func replay(tx *domain.Transaction, hash, _ string, _ *CreatePaymentRequest) (*CreatePaymentResponse, error) {
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
