package dto

import (
	"strings"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/utils"
)

// Party optionally identifies the person a payment is collected from or sent to.
//
// It is contextual rather than required: the account is what actually moves money, so
// a request may omit the party entirely.
type Party struct {
	// Name is the party's display name.
	Name string `json:"name" validate:"max=255"`
	// Email is the party's email address.
	Email string `json:"email" validate:"max=255"`
}

// Normalize trims the party's fields.
func (p *Party) Normalize() {
	if p == nil {
		return
	}
	p.Name, p.Email = strings.TrimSpace(p.Name), strings.TrimSpace(p.Email)
}

// CreatePayment contains the common fields used to initiate a collection or disbursement.
type CreatePayment struct {
	// PaymentMethod identifies the requested payment rail. It is free-form and must
	// match an active payment route.
	PaymentMethod string `json:"payment_method" validate:"required,identifier"`
	// Amount is the payment amount in the currency's minor unit.
	Amount int64 `json:"amount" validate:"gt=0"`
	// Currency is the three-letter currency code.
	Currency string `json:"currency" validate:"len=3"`
	// Country is the optional ISO 3166-1 alpha-2 transaction country. Providers that
	// declare supported countries are only eligible when it is present and matches.
	Country string `json:"country,omitempty" validate:"country"`
	// Reference is the application's unique business reference.
	Reference string `json:"reference" validate:"required,max=128"`
	// Description is optional payment context shown to downstream systems.
	Description string `json:"description" validate:"max=255"`
	// Account is the provider-specific account the payment is collected from or
	// disbursed to: a mobile number, bank account, card token, or wallet address.
	// Momobase checks its shape only and leaves its meaning to the selected provider,
	// through providers.RequestValidator.
	Account string `json:"account" validate:"required,account"`
	// Scheme optionally names the account's provider-specific scheme, such as a
	// mobile network, bank, or card brand. Like PaymentMethod it is free-form; the
	// selected provider interprets it and Momobase never matches on it.
	Scheme string `json:"scheme,omitempty" validate:"identifier"`
	// Metadata optionally carries provider-specific payment details, such as a bank
	// branch code. It reaches the selected provider and is never persisted, so it
	// cannot become a free-form store of identifiers Momobase would then have to
	// protect. It is part of the idempotency hash.
	Metadata map[string]any `json:"metadata,omitempty"`
	// Customer optionally identifies the collection customer.
	Customer *Party `json:"customer,omitempty"`
	// Recipient optionally identifies the disbursement recipient.
	Recipient *Party `json:"recipient,omitempty"`
}

// Normalize trims and cases the request into the form the engine compares and stores.
//
// This is what the idempotency hash is taken over, so it decides what counts as the
// same request: " ugx " and "UGX" are one, and a spelling this does not touch is two.
// The account is trimmed but never re-cased — case can be significant in a wallet
// address or a card token, and the provider is the one that knows.
func (r *CreatePayment) Normalize() {
	if r == nil {
		return
	}
	// An invalid country is left as it arrived for the country rule to reject; the
	// normalizer's error would otherwise be swallowed here and reported as valid.
	if country, err := utils.NormalizeOptionalCountry(r.Country); err == nil {
		r.Country = country
	}
	r.Currency = strings.ToUpper(strings.TrimSpace(r.Currency))
	r.PaymentMethod = strings.ToLower(strings.TrimSpace(r.PaymentMethod))
	r.Account = strings.TrimSpace(r.Account)
	r.Scheme = strings.ToLower(strings.TrimSpace(r.Scheme))
	r.Reference = strings.TrimSpace(r.Reference)
	r.Customer.Normalize()
	r.Recipient.Normalize()
}

// Party returns the party a service belongs to: the recipient of a disbursement, or
// the customer of a collection.
func (r *CreatePayment) Party(service string) *Party {
	if service == domain.ServiceDisbursement {
		return r.Recipient
	}
	return r.Customer
}
