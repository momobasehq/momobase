package domain

import (
	"errors"
	"fmt"
)

const (
	// ChargeFlat interprets a rule's value as a currency minor-unit amount.
	ChargeFlat = "flat"
	// ChargePercentage interprets a rule's value as basis points.
	ChargePercentage = "percentage"

	basisPointScale int64 = 10_000
)

// ChargeRule defines one fee charged for a payment service.
type ChargeRule struct {
	Type  string `gorm:"size:16;not null;default:flat" json:"type"`
	Value int64  `gorm:"not null;default:0" json:"value"`
}

// ChargeSchedule defines independent collection and disbursement fees.
type ChargeSchedule struct {
	Collection   ChargeRule `gorm:"embedded;embeddedPrefix:collection_charge_" json:"collection"`
	Disbursement ChargeRule `gorm:"embedded;embeddedPrefix:disbursement_charge_" json:"disbursement"`
}

// Normalize fills the zero-value representation with the persisted defaults.
func (s *ChargeSchedule) Normalize() {
	if s.Collection.Type == "" {
		s.Collection.Type = ChargeFlat
	}
	if s.Disbursement.Type == "" {
		s.Disbursement.Type = ChargeFlat
	}
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

// Validate rejects an unknown, negative, or excessive rule.
func (r ChargeRule) Validate() error {
	if r.Value < 0 {
		return errors.New("value must not be negative")
	}
	switch r.Type {
	case ChargeFlat:
		return nil
	case ChargePercentage:
		if r.Value > basisPointScale {
			return errors.New("percentage must not exceed 10000 basis points")
		}
		return nil
	default:
		return errors.New("type must be flat or percentage")
	}
}

// Calculate returns the fee for amount, rounded half-up for percentage rules.
func (r ChargeRule) Calculate(amount int64) (int64, error) {
	if amount < 0 {
		return 0, errors.New("amount must not be negative")
	}
	if err := r.Validate(); err != nil {
		return 0, err
	}
	if r.Type == ChargeFlat {
		return r.Value, nil
	}

	// Splitting the quotient and remainder avoids overflowing amount * basis points.
	whole := amount / basisPointScale
	remainder := amount % basisPointScale
	return whole*r.Value + (remainder*r.Value+basisPointScale/2)/basisPointScale, nil
}
