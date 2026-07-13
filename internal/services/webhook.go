package services

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"momobase/internal/domain"
	"momobase/internal/platform"
	"momobase/internal/providers"
	"momobase/internal/store"
)

type VerifiedWebhook struct {
	providers.ProviderWebhookEvent
	ProviderAccountID string `json:"provider_account_id"`
	PayloadHash       string `json:"payload_hash"`
}
type WebhookService struct {
	db       *gorm.DB
	runtime  *ProviderRuntimeManager
	executor *RuntimeProviderExecutor
}

func NewWebhookService(db *gorm.DB, runtime *ProviderRuntimeManager) *WebhookService {
	return &WebhookService{db, runtime, NewProviderExecutor(runtime)}
}
func (s *WebhookService) verify(ctx context.Context, accountID string, payload []byte, headers map[string]string) (*VerifiedWebhook, error) {
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
	event.Currency, event.Country, event.Raw = strings.ToUpper(event.Currency), strings.ToUpper(event.Country), redactRawMap(event.Raw)
	out := &VerifiedWebhook{ProviderWebhookEvent: *event, ProviderAccountID: accountID}
	out.PayloadHash = CanonicalWebhookHash(out)
	return out, nil
}
func (s *WebhookService) Handle(ctx context.Context, accountID string, payload []byte, headers map[string]string) error {
	event, err := s.verify(ctx, accountID, payload, headers)
	if err != nil {
		return err
	}
	stored, err := json.Marshal(event)
	if err != nil {
		return err
	}
	row := &domain.WebhookEvent{BaseModel: domain.BaseModel{ID: platform.NewID("wh")}, ProviderAccountID: accountID, ProviderReference: event.ProviderReference, EventType: event.EventType, PayloadHash: event.PayloadHash, PayloadJSON: string(stored)}
	return store.Within(ctx, s.db, func(db *gorm.DB) error {
		created := db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "provider_account_id"}, {Name: "payload_hash"}}, DoNothing: true}).Create(row)
		if created.Error != nil || created.RowsAffected == 0 {
			return created.Error
		}
		return s.apply(db, row, event)
	})
}
func (s *WebhookService) apply(db *gorm.DB, row *domain.WebhookEvent, event *VerifiedWebhook) error {
	tx, attempt, err := findWebhookTarget(db, event.ProviderAccountID, event.ProviderReference)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := validateWebhook(event, tx); err != nil {
		return err
	}
	if err := transition(tx, event.Status); err != nil {
		return err
	}
	now := time.Now().UTC()
	txUpdates := map[string]any{"status": tx.Status, "provider_reference": event.ProviderReference, "next_reconcile_at": nil}
	if !terminal(tx.Status) {
		next := now.Add(time.Minute)
		txUpdates["next_reconcile_at"] = &next
	}
	if err := store.Affected(db.Model(&domain.Transaction{}).Where("id = ?", tx.ID).Updates(txUpdates)); err != nil {
		return err
	}
	raw, _ := json.Marshal(event.Raw)
	attemptUpdates := map[string]any{"status": tx.Status, "provider_reference": event.ProviderReference, "raw_response": string(raw)}
	if terminal(tx.Status) {
		attemptUpdates["completed_at"] = &now
	}
	if err := store.Affected(db.Model(attempt).Updates(attemptUpdates)); err != nil {
		return err
	}
	return store.Affected(db.Model(row).Updates(map[string]any{"transaction_id": tx.ID, "processed": true}))
}
func (s *WebhookService) ReprocessPending(ctx context.Context, limit int) error {
	if limit < 1 {
		limit = 100
	}
	var rows []domain.WebhookEvent
	if err := s.db.WithContext(ctx).Where("processed = ?", false).Order("created_at asc").Limit(limit).Find(&rows).Error; err != nil {
		return err
	}
	for i := range rows {
		var event VerifiedWebhook
		if json.Unmarshal([]byte(rows[i].PayloadJSON), &event) != nil {
			continue
		}
		if err := store.Within(ctx, s.db, func(db *gorm.DB) error { return s.apply(db, &rows[i], &event) }); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}
	return nil
}
func findWebhookTarget(db *gorm.DB, accountID, ref string) (*domain.Transaction, *domain.TransactionAttempt, error) {
	var attempt domain.TransactionAttempt
	if err := db.Where("provider_account_id = ? AND provider_reference = ?", accountID, ref).Order("created_at desc").First(&attempt).Error; err != nil {
		return nil, nil, err
	}
	var tx domain.Transaction
	return &tx, &attempt, db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&tx, "id = ?", attempt.TransactionID).Error
}

func CanonicalWebhookHash(event *VerifiedWebhook) string {
	if event == nil {
		return ""
	}
	amount := ""
	if event.Amount != nil {
		amount = strconv.FormatInt(*event.Amount, 10)
	}
	return platform.SHA256Hex(strings.Join([]string{event.ProviderAccountID, event.ProviderReference, event.EventType, event.Status, event.ExternalReference, amount, strings.ToUpper(event.Currency)}, "|"))
}
func validateWebhook(event *VerifiedWebhook, tx *domain.Transaction) error {
	if event.Amount != nil && *event.Amount != tx.Amount || event.Currency != "" && !strings.EqualFold(event.Currency, tx.Currency) || event.Country != "" && !strings.EqualFold(event.Country, tx.Country) || event.ExternalReference != "" && event.ExternalReference != tx.Reference || event.Phone != "" && normalizeDigits(event.Phone) != normalizeDigits(tx.CustomerPhone) {
		return fmt.Errorf("webhook payload does not match transaction")
	}
	return nil
}

func normalizeDigits(v string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, v)
}
