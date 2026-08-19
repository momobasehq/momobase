package payment

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/utils"
)

func paymentParty(service string, req *CreatePaymentRequest) *PartyPayload {
	if service == domain.ServiceDisbursement {
		return req.Recipient
	}
	return req.Customer
}

// ValidatePaymentPayload validates a payment request and normalizes its country,
// currency, payment method, account, and scheme.
//
// The account is only checked for shape. What a usable account looks like is the
// selected provider's to decide, through providers.RequestValidator, so a request
// that is structurally sound here can still be rejected once a route is chosen.
func ValidatePaymentPayload(service string, req *CreatePaymentRequest) error {
	if req == nil {
		return errors.New("payment request is required")
	}
	country, err := utils.NormalizeOptionalCountry(req.Country)
	if err != nil {
		return err
	}
	req.Country, req.Currency, req.PaymentMethod =
		country,
		strings.ToUpper(strings.TrimSpace(req.Currency)),
		strings.ToLower(strings.TrimSpace(req.PaymentMethod))
	req.Account, req.Scheme =
		strings.TrimSpace(req.Account),
		strings.ToLower(strings.TrimSpace(req.Scheme))
	switch {
	case req.PaymentMethod == "" || !utils.ValidIdentifier(req.PaymentMethod):
		return errors.New("payment_method is required and may contain only letters, digits, and _-. and must not exceed 64 characters")
	case req.Amount <= 0:
		return errors.New("amount must be greater than zero")
	case len(req.Currency) != 3:
		return errors.New("currency must be a 3-letter code")
	case strings.TrimSpace(req.Reference) == "" || len(req.Reference) > 128:
		return errors.New("reference is required and must not exceed 128 characters")
	case len(req.Description) > 255:
		return errors.New("description must not exceed 255 characters")
	case req.Account == "":
		return errors.New("account is required")
	case !utils.ValidAccount(req.Account):
		return errors.New("account must not exceed 255 characters or contain control characters")
	case !utils.ValidIdentifier(req.Scheme):
		return errors.New("scheme may contain only letters, digits, and _-. and must not exceed 64 characters")
	}
	return validateParty(paymentParty(service, req))
}

// validateParty normalizes the optional party details. A party is contextual: the
// account is what a payment needs to move money, so a request may omit one.
func validateParty(party *PartyPayload) error {
	if party == nil {
		return nil
	}
	party.Name, party.Email = strings.TrimSpace(party.Name), strings.TrimSpace(party.Email)
	if len(party.Name) > 255 || len(party.Email) > 255 {
		return errors.New("customer or recipient name and email must not exceed 255 characters")
	}
	return nil
}

// PaymentRequestHash returns the canonical SHA-256 request hash used for idempotency checks.
func PaymentRequestHash(service string, req *CreatePaymentRequest) string {
	data, _ := json.Marshal(struct {
		Service string
		Request *CreatePaymentRequest
	}{service, req})
	return platform.SHA256Hex(string(data))
}
