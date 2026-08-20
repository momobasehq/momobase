package webhook

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/repository"
	"github.com/momobasehq/momobase/internal/service/provider"
	"github.com/momobasehq/momobase/internal/utils"
	"github.com/momobasehq/momobase/providers"
)

// verifiedWebhook contains a provider-verified event and its Momobase routing metadata.
type verifiedWebhook struct {
	// ProviderWebhookEvent is the normalized event returned by the provider adapter.
	providers.ProviderWebhookEvent
	// ProviderAccountID identifies the provider account that received the webhook.
	ProviderAccountID string `json:"provider_account_id"`
	// PayloadHash is the canonical event hash used to suppress duplicate processing.
	PayloadHash string `json:"payload_hash"`
}

// Service authenticates, verifies, stores, and applies provider webhook events.
type Service struct {
	repos    *repository.UnitOfWork
	runtime  *provider.RuntimeManager
	executor *provider.Executor
}

// New creates a provider webhook processing service.
func New(repos *repository.UnitOfWork, runtime *provider.RuntimeManager) *Service {
	return &Service{repos, runtime, provider.NewExecutor(runtime)}
}
func (s *Service) verify(
	ctx context.Context,
	accountID string,
	payload []byte,
	headers map[string]string,
) (*verifiedWebhook, error) {
	rp, ok := s.runtime.Get(accountID)
	if !ok || rp.WebhookSecret == "" {
		return nil, errors.New("provider webhook is not initialized")
	}
	secret := headers["X-Webhook-Secret"]
	if secret == "" {
		secret = headers["x-webhook-secret"]
	}
	if subtle.ConstantTimeCompare([]byte(secret), []byte(rp.WebhookSecret)) != 1 {
		return nil, errors.New("invalid webhook secret")
	}
	event, err := s.executor.VerifyWebhook(ctx, accountID, payload, headers)
	if err != nil {
		return nil, err
	}
	if event.ProviderReference == "" {
		return nil, errors.New("invalid provider reference")
	}
	event.Currency, event.Country, event.Raw =
		strings.ToUpper(event.Currency),
		strings.ToUpper(event.Country),
		utils.RedactRawMap(event.Raw)
	out := &verifiedWebhook{ProviderWebhookEvent: *event, ProviderAccountID: accountID}
	out.PayloadHash = canonicalWebhookHash(out)
	return out, nil
}

// Handle verifies and idempotently stores a webhook, then applies it to a matching transaction when available.
func (s *Service) Handle(ctx context.Context, accountID string, payload []byte, headers map[string]string) error {
	event, err := s.verify(ctx, accountID, payload, headers)
	if err != nil {
		return err
	}
	stored, err := json.Marshal(event)
	if err != nil {
		return err
	}
	row := &domain.WebhookEvent{
		BaseModel:         domain.BaseModel{ID: platform.NewID("wh")},
		ProviderAccountID: accountID,
		ProviderReference: event.ProviderReference,
		EventType:         event.EventType,
		PayloadHash:       event.PayloadHash,
		PayloadJSON:       string(stored),
	}
	return s.repos.Within(ctx, func(r *repository.Set) error {
		// A provider that delivers the same event twice inserts nothing the second
		// time, and applying it again is exactly what must not happen.
		inserted, err := r.WebhookEvents.Insert(ctx, row)
		if err != nil || !inserted {
			return err
		}
		return s.apply(ctx, r, row, event)
	})
}
func (s *Service) apply(
	ctx context.Context,
	r *repository.Set,
	row *domain.WebhookEvent,
	event *verifiedWebhook,
) error {
	tx, attempt, err := findWebhookTarget(ctx, r, event.ProviderAccountID, event.ProviderReference)
	if repository.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := validateWebhook(event, tx); err != nil {
		return err
	}
	if err := tx.Transition(event.Status); err != nil {
		return err
	}
	now := time.Now().UTC()
	txUpdates := map[string]any{"status": tx.Status, "provider_reference": event.ProviderReference, "next_reconcile_at": nil}
	if !domain.Terminal(tx.Status) {
		next := now.Add(time.Minute)
		txUpdates["next_reconcile_at"] = &next
	}
	if err := r.Transactions.Update(ctx, tx.ID, txUpdates); err != nil {
		return err
	}
	raw, _ := json.Marshal(event.Raw)
	attemptUpdates := map[string]any{"status": tx.Status, "provider_reference": event.ProviderReference, "raw_response": string(raw)}
	if domain.Terminal(tx.Status) {
		attemptUpdates["completed_at"] = &now
	}
	if err := r.TransactionAttempts.Update(ctx, attempt.ID, attemptUpdates); err != nil {
		return err
	}
	return r.WebhookEvents.MarkProcessed(ctx, row.ID, tx.ID)
}

// ReprocessPending retries applying up to limit stored webhook events that have not been processed.
func (s *Service) ReprocessPending(ctx context.Context, limit int) error {
	if limit < 1 {
		limit = 100
	}
	rows, err := s.repos.WebhookEvents.Pending(ctx, limit)
	if err != nil {
		return err
	}
	for i := range rows {
		var event verifiedWebhook
		if json.Unmarshal([]byte(rows[i].PayloadJSON), &event) != nil {
			continue
		}
		if err := s.repos.Within(ctx, func(r *repository.Set) error {
			return s.apply(ctx, r, &rows[i], &event)
		}); err != nil && !repository.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// findWebhookTarget resolves the transaction an inbound event belongs to, through the
// most recent attempt carrying the provider's reference, and locks it for the caller.
// Without the lock two deliveries of the same outcome could both apply.
func findWebhookTarget(
	ctx context.Context,
	r *repository.Set,
	accountID, ref string,
) (*domain.Transaction, *domain.TransactionAttempt, error) {
	attempt, err := r.TransactionAttempts.LatestForReference(ctx, accountID, ref)
	if err != nil {
		return nil, nil, err
	}
	tx, err := r.Transactions.LockForUpdate(ctx, attempt.TransactionID)
	if err != nil {
		return nil, nil, err
	}
	return tx, attempt, nil
}

// canonicalWebhookHash returns a stable SHA-256 hash of a verified webhook's identifying fields.
func canonicalWebhookHash(event *verifiedWebhook) string {
	if event == nil {
		return ""
	}
	amount := ""
	if event.Amount != nil {
		amount = strconv.FormatInt(*event.Amount, 10)
	}
	return platform.SHA256Hex(strings.Join([]string{
		event.ProviderAccountID,
		event.ProviderReference,
		event.EventType,
		event.Status,
		event.ExternalReference,
		amount,
		strings.ToUpper(event.Currency),
	}, "|"))
}
func validateWebhook(event *verifiedWebhook, tx *domain.Transaction) error {
	// The account is compared exactly: the provider reports the form it normalized
	// the request to, which is the form the transaction recorded. An event that
	// carries no account skips the check.
	if event.Amount != nil && *event.Amount != tx.Amount ||
		event.Currency != "" && !strings.EqualFold(event.Currency, tx.Currency) ||
		event.Country != "" && !strings.EqualFold(event.Country, tx.Country) ||
		event.ExternalReference != "" && event.ExternalReference != tx.Reference ||
		event.Account != "" && event.Account != tx.CustomerAccount {
		return fmt.Errorf("webhook payload does not match transaction")
	}
	return nil
}
