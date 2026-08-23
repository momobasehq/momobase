package domain

import "testing"

func TestChargeRuleCalculate(t *testing.T) {
	tests := []struct {
		name   string
		rule   ChargeRule
		amount int64
		want   int64
	}{
		{name: "zero flat", rule: ChargeRule{Type: ChargeFlat}, amount: 10, want: 0},
		{name: "flat", rule: ChargeRule{Type: ChargeFlat, Value: 250}, amount: 10, want: 250},
		{name: "percentage exact", rule: ChargeRule{Type: ChargePercentage, Value: 1_000}, amount: 1_000, want: 100},
		{name: "percentage rounds down", rule: ChargeRule{Type: ChargePercentage, Value: 1_000}, amount: 14, want: 1},
		{name: "percentage rounds half up", rule: ChargeRule{Type: ChargePercentage, Value: 1_000}, amount: 15, want: 2},
		{name: "maximum amount does not overflow", rule: ChargeRule{Type: ChargePercentage, Value: 10_000}, amount: int64(^uint64(0) >> 1), want: int64(^uint64(0) >> 1)},
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
		{Type: "tiered", Value: 1},
		{Type: ChargeFlat, Value: -1},
		{Type: ChargePercentage, Value: 10_001},
	} {
		if _, err := rule.Calculate(100); err == nil {
			t.Errorf("Calculate(%+v) succeeded", rule)
		}
	}
}
