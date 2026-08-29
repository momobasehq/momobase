package payment

import (
	"time"

	"github.com/momobasehq/momobase/internal/domain"
)

// AppTransaction is the app-facing transaction representation. ProviderFee is an
// internal platform cost and is deliberately absent.
type AppTransaction struct {
	ID                        string               `json:"id"`
	AppID                     string               `json:"app_id"`
	ServiceType               string               `json:"service_type"`
	PaymentMethod             domain.PaymentMethod `json:"payment_method"`
	Amount                    int64                `json:"amount"`
	Currency                  string               `json:"currency"`
	Country                   string               `json:"country"`
	Reference                 string               `json:"reference"`
	IdempotencyKey            string               `json:"idempotency_key"`
	Status                    string               `json:"status"`
	SelectedRouteID           string               `json:"selected_route_id"`
	SelectedProviderAccountID string               `json:"selected_provider_account_id"`
	ProviderReference         string               `json:"provider_reference"`
	CustomerAccount           string               `json:"customer_account"`
	CustomerEmail             string               `json:"customer_email"`
	CustomerName              string               `json:"customer_name"`
	Description               string               `json:"description"`
	PlatformFee               int64                `json:"platform_fee"`
	ReconciliationAttempts    int                  `json:"reconciliation_attempts"`
	LastReconciledAt          *time.Time           `json:"last_reconciled_at"`
	NextReconcileAt           *time.Time           `json:"next_reconcile_at"`
	CreatedAt                 time.Time            `json:"created_at"`
	UpdatedAt                 time.Time            `json:"updated_at"`
}

// PublicTransaction removes internal provider pricing from a persisted transaction.
func PublicTransaction(tx *domain.Transaction) AppTransaction {
	return AppTransaction{
		ID:                        tx.ID,
		AppID:                     tx.AppID,
		ServiceType:               tx.ServiceType,
		PaymentMethod:             tx.PaymentMethod,
		Amount:                    tx.Amount,
		Currency:                  tx.Currency,
		Country:                   tx.Country,
		Reference:                 tx.Reference,
		IdempotencyKey:            tx.IdempotencyKey,
		Status:                    tx.Status,
		SelectedRouteID:           tx.SelectedRouteID,
		SelectedProviderAccountID: tx.SelectedProviderAccountID,
		ProviderReference:         tx.ProviderReference,
		CustomerAccount:           tx.CustomerAccount,
		CustomerEmail:             tx.CustomerEmail,
		CustomerName:              tx.CustomerName,
		Description:               tx.Description,
		PlatformFee:               tx.PlatformFee,
		ReconciliationAttempts:    tx.ReconciliationAttempts,
		LastReconciledAt:          tx.LastReconciledAt,
		NextReconcileAt:           tx.NextReconcileAt,
		CreatedAt:                 tx.CreatedAt,
		UpdatedAt:                 tx.UpdatedAt,
	}
}
