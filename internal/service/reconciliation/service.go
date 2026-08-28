package reconciliation

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/momobasehq/momobase/hooks"
	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/repository"
	"github.com/momobasehq/momobase/internal/service/provider"
	"github.com/momobasehq/momobase/internal/service/webhook"
	"github.com/momobasehq/momobase/providers"
)

// Service refreshes non-terminal transaction states and retries pending webhook processing.
type Service struct {
	repos    *repository.UnitOfWork
	executor *provider.Executor
	webhook  *webhook.Service
	logger   *slog.Logger
	hooks    *hooks.Registry
}

// Deps contains the collaborators used by the reconciliation service.
type Deps struct {
	// Repos provides transaction-bound persistence.
	Repos *repository.UnitOfWork
	// Runtime provides initialized payment providers.
	Runtime *provider.RuntimeManager
	// Webhook retries verified events that arrived before their transaction.
	Webhook *webhook.Service
	// Logger records reconciliation failures.
	Logger *slog.Logger
	// Hooks publishes committed transaction status changes.
	Hooks *hooks.Registry
}

// New creates a transaction reconciliation service.
func New(d Deps) *Service {
	return &Service{
		repos:    d.Repos,
		executor: provider.NewExecutor(d.Runtime),
		webhook:  d.Webhook,
		logger:   d.Logger,
		hooks:    d.Hooks,
	}
}

// RunOnce reconciles up to limit eligible transactions and reprocesses pending webhooks.
func (s *Service) RunOnce(ctx context.Context, limit int) error {
	if limit < 1 {
		limit = 100
	}
	rows, err := s.repos.Transactions.DueForReconciliation(ctx, time.Now().UTC(), limit)
	if err != nil {
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
	var change *hooks.TransactionChangedEvent
	err = s.repos.Within(ctx, func(r *repository.Set) error {
		// Re-read under a lock: the request path or a webhook may have settled this
		// transaction between selecting it and asking the provider about it.
		tx, err := r.Transactions.LockForUpdate(ctx, row.ID)
		if err != nil || domain.Terminal(tx.Status) {
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
			"reconciliation_attempts": repository.CountReconcileAttempt(),
			"next_reconcile_at":       nil,
		}
		if !domain.Terminal(tx.Status) {
			next := now.Add(backoff(tx.ReconciliationAttempts + 1))
			updates["next_reconcile_at"] = &next
		}
		// Conditioned on the status the row was read at, so an outcome that landed
		// first is never overwritten by this one.
		applied, err := r.Transactions.UpdateFromStatus(ctx, tx.ID, previous, updates)
		if err != nil || !applied {
			return err
		}
		attempt, err := r.TransactionAttempts.LatestForTransaction(ctx, tx.ID, tx.SelectedProviderAccountID)
		if err != nil {
			return err
		}
		attemptUpdates := map[string]any{"status": tx.Status, "error_code": "", "error_message": ""}
		if domain.Terminal(tx.Status) {
			attemptUpdates["completed_at"] = &now
		}
		if err := r.TransactionAttempts.Update(ctx, attempt.ID, attemptUpdates); err != nil {
			return err
		}
		if previous != tx.Status {
			change = transactionChangedEvent(hooks.TransactionSourceReconciliation, previous, tx)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if change != nil {
		s.hooks.NotifyTransactionChanged(ctx, *change)
	}
	return nil
}
func (s *Service) deferRetry(ctx context.Context, row *domain.Transaction, cause error) error {
	var change *hooks.TransactionChangedEvent
	err := s.repos.Within(ctx, func(r *repository.Set) error {
		tx, err := r.Transactions.LockForUpdate(ctx, row.ID)
		if err != nil || domain.Terminal(tx.Status) {
			return err
		}
		previous, now := tx.Status, time.Now().UTC()
		if err := tx.Transition(domain.TxUnknown); err != nil {
			return err
		}
		next := now.Add(backoff(tx.ReconciliationAttempts + 1))
		if _, err := r.Transactions.UpdateFromStatus(ctx, tx.ID, previous, map[string]any{
			"status":                  tx.Status,
			"last_reconciled_at":      &now,
			"next_reconcile_at":       &next,
			"reconciliation_attempts": repository.CountReconcileAttempt(),
		}); err != nil {
			return err
		}
		if previous != tx.Status {
			change = transactionChangedEvent(hooks.TransactionSourceReconciliation, previous, tx)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if change != nil {
		s.hooks.NotifyTransactionChanged(ctx, *change)
	}
	return cause
}

func transactionChangedEvent(
	source string,
	previous string,
	tx *domain.Transaction,
) *hooks.TransactionChangedEvent {
	return &hooks.TransactionChangedEvent{
		Source:            source,
		AppID:             tx.AppID,
		TransactionID:     tx.ID,
		Reference:         tx.Reference,
		ServiceType:       tx.ServiceType,
		PaymentMethod:     tx.PaymentMethod,
		Amount:            tx.Amount,
		Currency:          tx.Currency,
		Country:           tx.Country,
		PreviousStatus:    previous,
		Status:            tx.Status,
		ProviderAccountID: tx.SelectedProviderAccountID,
		ProviderReference: tx.ProviderReference,
	}
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
