package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/nyaruka/phonenumbers"
	"gorm.io/gorm"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/providers"
	"github.com/momobasehq/momobase/internal/store"
)

type PartyPayload struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Phone   string `json:"phone"`
	Network string `json:"network,omitempty"`
}
type CreatePaymentRequest struct {
	PaymentMethod string        `json:"payment_method"`
	Amount        int64         `json:"amount"`
	Currency      string        `json:"currency"`
	Country       string        `json:"country"`
	Reference     string        `json:"reference"`
	Description   string        `json:"description"`
	Customer      *PartyPayload `json:"customer,omitempty"`
	Recipient     *PartyPayload `json:"recipient,omitempty"`
	Momo          *PartyPayload `json:"momo,omitempty"`
}
type CreatePaymentResponse struct {
	TransactionID     string `json:"transaction_id"`
	Reference         string `json:"reference"`
	ServiceType       string `json:"service_type"`
	PaymentMethod     string `json:"payment_method"`
	Status            string `json:"status"`
	SelectedProvider  string `json:"selected_provider"`
	ProviderReference string `json:"provider_reference"`
	Message           string `json:"message"`
}
type PaymentOrchestrator struct {
	db       *gorm.DB
	router   *RouteEngine
	executor *RuntimeProviderExecutor
}

func NewPaymentOrchestrator(db *gorm.DB, router *RouteEngine, executor *RuntimeProviderExecutor) *PaymentOrchestrator {
	return &PaymentOrchestrator{db, router, executor}
}
func paymentParty(service string, req *CreatePaymentRequest) *PartyPayload {
	if service == domain.ServiceDisbursement {
		return req.Recipient
	}
	return req.Customer
}
func ValidatePaymentPayload(service string, req *CreatePaymentRequest) error {
	if req == nil {
		return errors.New("payment request is required")
	}
	country, err := NormalizeTransactionCountry(req.Country)
	if err != nil {
		return err
	}
	req.Country, req.Currency, req.PaymentMethod = country, strings.ToUpper(strings.TrimSpace(req.Currency)), strings.ToLower(strings.TrimSpace(req.PaymentMethod))
	if req.Momo != nil {
		req.Momo.Network = strings.ToLower(strings.TrimSpace(req.Momo.Network))
	}
	switch {
	case req.PaymentMethod != domain.PaymentMethodMomo:
		return errors.New("only payment_method=momo is implemented")
	case req.Amount <= 0:
		return errors.New("amount must be greater than zero")
	case len(req.Currency) != 3:
		return errors.New("currency must be a 3-letter code")
	case strings.TrimSpace(req.Reference) == "" || len(req.Reference) > 128:
		return errors.New("reference is required and must not exceed 128 characters")
	case len(req.Description) > 255:
		return errors.New("description must not exceed 255 characters")
	case req.Momo == nil || strings.TrimSpace(req.Momo.Phone) == "":
		return errors.New("momo.phone is required")
	case req.Momo.Network != "" && req.Momo.Network != "mtn" && req.Momo.Network != "airtel" && req.Momo.Network != "unknown":
		return errors.New("momo.network must be mtn, airtel, or unknown")
	}
	party := paymentParty(service, req)
	if party == nil || strings.TrimSpace(party.Phone) == "" {
		return errors.New("customer or recipient with phone is required")
	}
	phone, err := NormalizeMSISDN(req.Momo.Phone, country)
	if err == nil {
		req.Momo.Phone, party.Phone = phone, phone
	}
	return err
}
func (o *PaymentOrchestrator) Create(ctx context.Context, appID, service, key string, req *CreatePaymentRequest) (*CreatePaymentResponse, error) {
	if err := ValidatePaymentPayload(service, req); err != nil {
		return nil, err
	}
	if strings.TrimSpace(key) == "" {
		return nil, errors.New("Idempotency-Key header is required")
	}
	hash := PaymentRequestHash(service, req)
	if tx, err := o.find(ctx, appID, key); err == nil {
		return replay(tx, hash, service, req)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	selected, err := o.router.SelectProvider(ctx, service, req.PaymentMethod, req.Country)
	if err != nil {
		return nil, err
	}
	party := paymentParty(service, req)
	tx := &domain.Transaction{BaseModel: domain.BaseModel{ID: platform.NewID("txn")}, AppID: appID, ServiceType: service, PaymentMethod: req.PaymentMethod, Amount: req.Amount, Currency: req.Currency, Country: req.Country, Reference: req.Reference, IdempotencyKey: key, Status: domain.TxProcessing, SelectedRouteID: selected.Route.ID, SelectedProviderAccountID: selected.Account.ID, CustomerPhone: party.Phone, CustomerEmail: party.Email, CustomerName: party.Name, Description: req.Description, RequestHash: hash}
	attempt := &domain.TransactionAttempt{BaseModel: domain.BaseModel{ID: platform.NewID("att")}, TransactionID: tx.ID, ProviderAccountID: selected.Account.ID, ProviderCode: selected.Runtime.ProviderCode, Status: domain.TxProcessing, RequestHash: hash, StartedAt: time.Now().UTC()}
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
	call := providers.PaymentRequest{TransactionID: tx.ID, Amount: req.Amount, Currency: req.Currency, Country: req.Country, Reference: req.Reference, Phone: req.Momo.Phone, Network: req.Momo.Network, Description: req.Description}
	var result *providers.ProviderPaymentResponse
	if service == domain.ServiceCollection {
		result, err = o.executor.Collect(ctx, selected.Account.ID, call)
	} else {
		result, err = o.executor.Disburse(ctx, selected.Account.ID, call)
	}
	return o.persist(ctx, tx, attempt, selected.Runtime.ProviderCode, result, err)
}
func (o *PaymentOrchestrator) persist(ctx context.Context, tx *domain.Transaction, attempt *domain.TransactionAttempt, providerCode string, result *providers.ProviderPaymentResponse, cause error) (*CreatePaymentResponse, error) {
	now := time.Now().UTC()
	attemptUpdates := map[string]any{}
	txUpdates := map[string]any{"next_reconcile_at": nil}
	message := "provider response unknown"
	if cause != nil || result == nil {
		if cause == nil {
			cause = errors.New("provider returned no response")
		}
		if err := transition(tx, domain.TxUnknown); err != nil {
			return nil, err
		}
		attemptUpdates["status"], txUpdates["status"] = tx.Status, tx.Status
		attemptUpdates["error_code"], attemptUpdates["error_message"] = "PROVIDER_ERROR", providers.Redact(cause.Error())
	} else {
		target := providers.PaymentStatus(result.Status)
		if err := transition(tx, target); err != nil {
			return nil, err
		}
		tx.ProviderReference, message = result.ProviderReference, result.Message
		raw, _ := json.Marshal(redactRawMap(result.Raw))
		attemptUpdates["status"], attemptUpdates["provider_reference"], attemptUpdates["raw_response"] = tx.Status, tx.ProviderReference, string(raw)
		txUpdates["status"], txUpdates["provider_reference"] = tx.Status, tx.ProviderReference
	}
	if terminal(tx.Status) {
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
	return &CreatePaymentResponse{TransactionID: tx.ID, Reference: tx.Reference, ServiceType: tx.ServiceType, PaymentMethod: tx.PaymentMethod, Status: tx.Status, SelectedProvider: providerCode, ProviderReference: tx.ProviderReference, Message: message}, nil
}
func (o *PaymentOrchestrator) find(ctx context.Context, appID, key string) (*domain.Transaction, error) {
	var tx domain.Transaction
	return &tx, o.db.WithContext(ctx).Where("app_id = ? AND idempotency_key = ?", appID, key).First(&tx).Error
}
func replay(tx *domain.Transaction, hash, _ string, _ *CreatePaymentRequest) (*CreatePaymentResponse, error) {
	if tx.RequestHash != hash {
		return nil, errors.New("idempotency key reused with a different request")
	}
	return &CreatePaymentResponse{TransactionID: tx.ID, Reference: tx.Reference, ServiceType: tx.ServiceType, PaymentMethod: tx.PaymentMethod, Status: tx.Status, ProviderReference: tx.ProviderReference, Message: "idempotent replay"}, nil
}
func PaymentRequestHash(service string, req *CreatePaymentRequest) string {
	data, _ := json.Marshal(struct {
		Service string
		Request *CreatePaymentRequest
	}{service, req})
	return platform.SHA256Hex(string(data))
}
func NormalizeMSISDN(phone, country string) (string, error) {
	country, err := NormalizeTransactionCountry(country)
	if err != nil {
		return "", err
	}
	raw := strings.TrimSpace(phone)
	if raw == "" || strings.IndexFunc(raw, unicode.IsLetter) >= 0 {
		return "", errors.New("momo phone must contain only digits and phone punctuation")
	}
	digits := phonenumbers.NormalizeDigitsOnly(raw)
	callingCode := strconv.Itoa(phonenumbers.GetCountryCodeForRegion(country))
	if !strings.HasPrefix(raw, "+") && strings.HasPrefix(digits, callingCode) {
		raw = "+" + digits
	}
	number, err := phonenumbers.Parse(raw, country)
	if err != nil || !phonenumbers.IsValidNumberForRegion(number, country) {
		return "", errors.New("momo phone must be valid for the transaction country")
	}
	typeOfNumber := phonenumbers.GetNumberType(number)
	if typeOfNumber != phonenumbers.MOBILE && typeOfNumber != phonenumbers.FIXED_LINE_OR_MOBILE {
		return "", errors.New("momo phone must be a mobile number")
	}
	return strings.TrimPrefix(phonenumbers.Format(number, phonenumbers.E164), "+"), nil
}

func redactRawMap(raw map[string]any) map[string]any {
	out := make(map[string]any, len(raw))
	for k, v := range raw {
		key := strings.ToLower(k)
		if strings.Contains(key, "token") || strings.Contains(key, "secret") || strings.Contains(key, "key") || strings.Contains(key, "password") {
			v = "[redacted]"
		}
		out[k] = v
	}
	return out
}
