package services

import (
	"fmt"

	"github.com/momobasehq/momobase/internal/domain"
)

func transition(tx *domain.Transaction, next string) error {
	if tx == nil {
		return fmt.Errorf("transaction is nil")
	}
	current := tx.Status
	if current == "" {
		current = domain.TxPending
	}
	if current == next {
		return nil
	}
	valid := false
	switch current {
	case domain.TxPending:
		valid = next == domain.TxProcessing || terminal(next) || next == domain.TxUnknown
	case domain.TxProcessing:
		valid = terminal(next) || next == domain.TxUnknown
	case domain.TxUnknown:
		valid = next == domain.TxProcessing || terminal(next)
	}
	if !valid {
		return fmt.Errorf("illegal transaction transition %s -> %s", current, next)
	}
	tx.Status = next
	return nil
}
func terminal(status string) bool {
	return status == domain.TxSucceeded || status == domain.TxFailed || status == domain.TxExpired || status == domain.TxCancelled
}
