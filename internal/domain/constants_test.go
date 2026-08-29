package domain

import "testing"

func TestPaymentMethods(t *testing.T) {
	methods := PaymentMethods()
	if len(methods) != 4 {
		t.Fatalf("PaymentMethods() = %v, want four methods", methods)
	}
	for _, method := range methods {
		if !ValidPaymentMethod(method) {
			t.Errorf("ValidPaymentMethod(%q) = false", method)
		}
	}
	if ValidPaymentMethod("cash") {
		t.Error("ValidPaymentMethod(cash) = true")
	}
}
