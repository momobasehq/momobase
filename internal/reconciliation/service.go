package reconciliation

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/provider"
	"github.com/momobasehq/momobase/internal/store"
	"github.com/momobasehq/momobase/internal/webhook"
	"github.com/momobasehq/momobase/providers"
)

// Service refreshes non-terminal transaction states and retries pending webhook processing.
type Service struct {
	db       *gorm.DB
	executor *provider.Executor
	webhook  *webhook.Service
	logger   *slog.Logger
}

// New creates a transaction reconciliation service.
func New(
	db *gorm.DB,
	runtime *provider.RuntimeManager,
	webhook *webhook.Service,
	logger *slog.Logger,
) *Service {
	return &Service{db, provider.NewExecutor(runtime), webhook, logger}
}

// RunOnce reconciles up to limit eligible transactions and reprocesses pending webhooks.
func (s *Service) RunOnce(ctx context.Context, limit int) error {
	if limit < 1 {
		limit = 100
	}
	var rows []domain.Transaction
	now := time.Now().UTC()
	if err := s.db.WithContext(ctx).
		Where(
			"status IN ? AND provider_reference <> '' AND (next_reconcile_at IS NULL OR next_reconcile_at <= ?)",
			[]string{domain.TxPending, domain.TxProcessing, domain.TxUnknown},
			now,
		).
		Order("created_at asc").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return err
	}
	var errs []error
	for i := range rows {
		if err := s.reconcile(ctx, &rows[i]); err != nil {
			errs = append(errs, err)
			if s.logger != nil {
				s.logger.Warn(
					"reconciliation failed",
					slog.String("transaction_id", rows[i].ID),
					slog.String("error", providers.Redact(err.Error())),
				)
			}
		}
	}
	if s.webhook != nil {
		if err := s.webhook.ReprocessPending(ctx, limit); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
func (s *Service) reconcile(ctx context.Context, row *domain.Transaction) error {
	result, err := s.executor.QueryTransaction(ctx, row.SelectedProviderAccountID, row.ProviderReference, row.Country)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return s.deferRetry(ctx, row, err)
	}
	target := providers.PaymentStatus(result.Status)
	return store.Within(ctx, s.db, func(db *gorm.DB) error {
		var tx domain.Transaction
		if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&tx, "id = ?", row.ID).Error; err != nil || domain.Terminal(tx.Status) {
			return err
		}
		previous := tx.Status
		if err := tx.Transition(target); err != nil {
			return err
		}
		now := time.Now().UTC()
		updates := map[string]any{
			"status":                  tx.Status,
			"last_reconciled_at":      &now,
			"reconciliation_attempts": gorm.Expr("reconciliation_attempts + 1"),
			"next_reconcile_at":       nil,
		}
		if !domain.Terminal(tx.Status) {
			next := now.Add(backoff(tx.ReconciliationAttempts + 1))
			updates["next_reconcile_at"] = &next
		}
		updated := db.Model(&domain.Transaction{}).Where("id = ? AND status = ?", tx.ID, previous).Updates(updates)
		if updated.Error != nil || updated.RowsAffected == 0 {
			return updated.Error
		}
		var attempt domain.TransactionAttempt
		if err := db.Where(
			"transaction_id = ? AND provider_account_id = ?",
			tx.ID,
			tx.SelectedProviderAccountID,
		).Order("created_at desc").First(&attempt).Error; err != nil {
			return err
		}
		attemptUpdates := map[string]any{"status": tx.Status, "error_code": "", "error_message": ""}
		if domain.Terminal(tx.Status) {
			attemptUpdates["completed_at"] = &now
		}
		return store.Affected(db.Model(&attempt).Updates(attemptUpdates))
	})
}
func (s *Service) deferRetry(ctx context.Context, row *domain.Transaction, cause error) error {
	err := store.Within(ctx, s.db, func(db *gorm.DB) error {
		var tx domain.Transaction
		if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&tx, "id = ?", row.ID).Error; err != nil || domain.Terminal(tx.Status) {
			return err
		}
		previous, now := tx.Status, time.Now().UTC()
		if err := tx.Transition(domain.TxUnknown); err != nil {
			return err
		}
		next := now.Add(backoff(tx.ReconciliationAttempts + 1))
		result := db.Model(&domain.Transaction{}).
			Where("id = ? AND status = ?", tx.ID, previous).
			Updates(map[string]any{
				"status":                  tx.Status,
				"last_reconciled_at":      &now,
				"next_reconcile_at":       &next,
				"reconciliation_attempts": gorm.Expr("reconciliation_attempts + 1"),
			})
		if result.Error != nil || result.RowsAffected == 0 {
			return result.Error
		}
		return nil
	})
	if err != nil {
		return err
	}
	return cause
}
func backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 6 {
		attempt = 6
	}
	return time.Duration(1<<uint(attempt-1)) * time.Minute
}
