package domain

import (
	"errors"
	"fmt"
	"math"
)

const basisPointScale int64 = 10_000

// ChargeRule defines one fee charged for a payment service. Providers commonly bill
// a fixed amount plus a share of the transaction, so a rule carries both parts.
type ChargeRule struct {
	// Flat is a fixed fee in currency minor units, charged on every transaction.
	Flat int64 `gorm:"not null;default:0" json:"flat"`
	// Percentage is a proportional fee in basis points (100 = 1%).
	Percentage int64 `gorm:"not null;default:0" json:"percentage"`
}

// ChargeSchedule defines independent collection and disbursement fees.
type ChargeSchedule struct {
	Collection   ChargeRule `gorm:"embedded;embeddedPrefix:collection_charge_" json:"collection"`
	Disbursement ChargeRule `gorm:"embedded;embeddedPrefix:disbursement_charge_" json:"disbursement"`
}

// Validate rejects a schedule that cannot be calculated safely.
func (s ChargeSchedule) Validate() error {
	if err := s.Collection.Validate(); err != nil {
		return fmt.Errorf("collection charge: %w", err)
	}
	if err := s.Disbursement.Validate(); err != nil {
		return fmt.Errorf("disbursement charge: %w", err)
	}
	return nil
}

// Rule returns the charge configured for service.
func (s ChargeSchedule) Rule(service string) ChargeRule {
	if service == ServiceDisbursement {
		return s.Disbursement
	}
	return s.Collection
}

// Validate rejects a negative or excessive rule.
func (r ChargeRule) Validate() error {
	if r.Flat < 0 {
		return errors.New("flat must not be negative")
	}
	if r.Percentage < 0 {
		return errors.New("percentage must not be negative")
	}
	if r.Percentage > basisPointScale {
		return errors.New("percentage must not exceed 10000 basis points")
	}
	return nil
}

// Calculate returns the flat fee plus the percentage of amount, the latter rounded half-up.
func (r ChargeRule) Calculate(amount int64) (int64, error) {
	if amount < 0 {
		return 0, errors.New("amount must not be negative")
	}
	if err := r.Validate(); err != nil {
		return 0, err
	}

	// Splitting the quotient and remainder avoids overflowing amount * basis points.
	whole := amount / basisPointScale
	remainder := amount % basisPointScale
	proportional := whole*r.Percentage + (remainder*r.Percentage+basisPointScale/2)/basisPointScale
	if proportional > math.MaxInt64-r.Flat {
		return 0, errors.New("charge exceeds the largest representable amount")
	}
	return r.Flat + proportional, nil
}
