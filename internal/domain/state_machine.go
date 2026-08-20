package domain

import "fmt"

// Transition advances a transaction to the next status, rejecting any move the
// payment state graph does not allow.
//
// This is the only sanctioned way to change Transaction.Status: writing the field
// directly bypasses the graph and is a silent correctness bug. A transaction with no
// status yet is treated as pending, and a move to the current status is a no-op so
// callers may re-apply a status they already observed.
func (t *Transaction) Transition(next string) error {
	if t == nil {
		return fmt.Errorf("transaction is nil")
	}
	current := t.Status
	if current == "" {
		current = TxPending
	}
	if current == next {
		return nil
	}
	valid := false
	switch current {
	case TxPending:
		valid = next == TxProcessing || Terminal(next) || next == TxUnknown
	case TxProcessing:
		valid = Terminal(next) || next == TxUnknown
	case TxUnknown:
		valid = next == TxProcessing || Terminal(next)
	}
	if !valid {
		return fmt.Errorf("illegal transaction transition %s -> %s", current, next)
	}
	t.Status = next
	return nil
}

// Terminal reports whether a transaction status is final and admits no further
// transition. It takes a bare status rather than a transaction because the state
// graph also tests candidate statuses that no transaction carries yet.
func Terminal(status string) bool {
	return status == TxSucceeded || status == TxFailed || status == TxExpired || status == TxCancelled
}
