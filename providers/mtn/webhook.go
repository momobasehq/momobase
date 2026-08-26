package mtn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/providers"
)

// VerifyWebhook validates and normalizes an MTN MoMo callback payload. Momobase
// authenticates X-Webhook-Secret before this method is called.
func (p *Provider) VerifyWebhook(
	_ context.Context,
	payload []byte,
	_ map[string]string,
) (*providers.ProviderWebhookEvent, error) {
	if p.config().WebhookSecret == "" {
		return nil, errors.New("mtn: provider is not initialized")
	}
	var body transactionStatus
	if err := json.Unmarshal(payload, &body); err != nil {
		return nil, fmt.Errorf("mtn: decode webhook: %w", err)
	}
	if strings.TrimSpace(body.Status) == "" {
		return nil, errors.New("mtn: webhook status is required")
	}
	service, account, err := webhookParty(body)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(body.ExternalID)
	if err != nil || id.Version() != 4 {
		return nil, errors.New("mtn: webhook externalId is not a valid request reference")
	}
	amount, err := providers.OptionalAmount(body.Amount, body.Currency)
	if err != nil {
		return nil, fmt.Errorf("mtn: parse webhook amount: %w", err)
	}
	normalizedAccount, err := normalizeMSISDN(account)
	if err != nil {
		return nil, fmt.Errorf("mtn: webhook %w", err)
	}
	raw := map[string]any{}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, fmt.Errorf("mtn: decode webhook payload: %w", err)
	}
	return &providers.ProviderWebhookEvent{
		ProviderReference: makeReference(service, id.String()),
		Status:            providers.PaymentStatus(body.Status),
		EventType:         "mtn_momo." + service + ".updated",
		Amount:            amount,
		Currency:          strings.ToUpper(body.Currency),
		Account:           normalizedAccount,
		Raw:               raw,
	}, nil
}

func webhookParty(body transactionStatus) (string, string, error) {
	if body.Payer != nil && strings.EqualFold(body.Payer.PartyIDType, "MSISDN") && body.Payer.PartyID != "" && body.Payee == nil {
		return domain.ServiceCollection, body.Payer.PartyID, nil
	}
	if body.Payee != nil && strings.EqualFold(body.Payee.PartyIDType, "MSISDN") && body.Payee.PartyID != "" && body.Payer == nil {
		return domain.ServiceDisbursement, body.Payee.PartyID, nil
	}
	return "", "", errors.New("mtn: webhook must identify exactly one payer or payee")
}
