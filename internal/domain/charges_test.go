package domain

import (
	"math"
	"testing"
)

func TestChargeRuleCalculate(t *testing.T) {
	tests := []struct {
		name   string
		rule   ChargeRule
		amount int64
		want   int64
	}{
		{name: "zero", rule: ChargeRule{}, amount: 10, want: 0},
		{name: "flat", rule: ChargeRule{Flat: 250}, amount: 10, want: 250},
		{name: "percentage exact", rule: ChargeRule{Percentage: 1_000}, amount: 1_000, want: 100},
		{name: "percentage rounds down", rule: ChargeRule{Percentage: 1_000}, amount: 14, want: 1},
		{name: "percentage rounds half up", rule: ChargeRule{Percentage: 1_000}, amount: 15, want: 2},
		{name: "flat plus percentage", rule: ChargeRule{Flat: 500, Percentage: 300}, amount: 10_000, want: 800},
		{name: "flat plus percentage on zero amount", rule: ChargeRule{Flat: 500, Percentage: 300}, amount: 0, want: 500},
		{
			name:   "maximum amount does not overflow",
			rule:   ChargeRule{Percentage: 10_000},
			amount: math.MaxInt64,
			want:   math.MaxInt64,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.rule.Calculate(test.amount)
			if err != nil {
				t.Fatalf("Calculate() error = %v", err)
			}
			if got != test.want {
				t.Errorf("Calculate() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestChargeRuleRejectsInvalidValues(t *testing.T) {
	for _, rule := range []ChargeRule{
		{Flat: -1},
		{Percentage: -1},
		{Percentage: 10_001},
	} {
		if _, err := rule.Calculate(100); err == nil {
			t.Errorf("Calculate(%+v) succeeded", rule)
		}
	}
}

func TestChargeRuleRejectsAnOverflowingSum(t *testing.T) {
	rule := ChargeRule{Flat: 1, Percentage: 10_000}
	if _, err := rule.Calculate(math.MaxInt64); err == nil {
		t.Error("Calculate() succeeded, want an overflow error")
	}
}
