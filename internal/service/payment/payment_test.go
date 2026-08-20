package payment_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/service/payment"
	"github.com/momobasehq/momobase/internal/testsupport"
	"github.com/momobasehq/momobase/providers"
)

func TestValidatePaymentPayload(t *testing.T) {
	t.Run("normalizes the method, currency, country, and scheme", func(t *testing.T) {
		req := &payment.CreatePaymentRequest{
			PaymentMethod: "  MOMO ",
			Amount:        100,
			Currency:      " ugx ",
			Country:       " ug ",
			Reference:     "ORDER-1",
			Account:       "  256770000000 ", Scheme: " MTN ",
			Customer: &payment.PartyPayload{Name: "  Ada  "},
		}
		if err := payment.ValidatePaymentPayload(domain.ServiceCollection, req); err != nil {
			t.Fatalf("payment.ValidatePaymentPayload() error = %v", err)
		}
		if req.PaymentMethod != "momo" || req.Currency != "UGX" || req.Country != "UG" {
			t.Errorf("normalized request = %+v, want a lowercase method and uppercase currency and country", req)
		}
		if req.Account != "256770000000" || req.Scheme != "mtn" {
			t.Errorf("normalized account = %+v, want a trimmed account and a lowercase scheme", req.Account)
		}
		if req.Customer.Name != "Ada" {
			t.Errorf("normalized customer name = %q, want it trimmed", req.Customer.Name)
		}
	})

	t.Run("accepts an opaque account without a country or a party", func(t *testing.T) {
		req := testsupport.PaymentRequest("ORDER-2", "", "GB33BUKB20201555555555")
		if err := payment.ValidatePaymentPayload(domain.ServiceCollection, req); err != nil {
			t.Fatalf("payment.ValidatePaymentPayload() error = %v, want a rail-agnostic account accepted", err)
		}
		if req.Country != "" {
			t.Errorf("Country = %q, want it left empty", req.Country)
		}
	})

	t.Run("rejects a malformed request", func(t *testing.T) {
		tests := map[string]*payment.CreatePaymentRequest{
			"missing account":  {PaymentMethod: testsupport.Method, Amount: 1, Currency: "UGX", Reference: "R"},
			"blank account":    testsupport.PaymentRequest("R", "", "   "),
			"control account":  testsupport.PaymentRequest("R", "", "2567700\x0000"),
			"oversize account": testsupport.PaymentRequest("R", "", strings.Repeat("9", 256)),
			"unknown country":  testsupport.PaymentRequest("R", "XX", "256770000000"),
			"missing method":   {Amount: 1, Currency: "UGX", Reference: "R", Account: "1"},
			"bad method": {
				PaymentMethod: "bank transfer",
				Amount:        1,
				Currency:      "UGX",
				Reference:     "R",
				Account:       "1",
			},
			"bad scheme": {
				PaymentMethod: testsupport.Method,
				Amount:        1,
				Currency:      "UGX",
				Reference:     "R",
				Account:       "1", Scheme: "mtn/ug",
			},
			"zero amount": {PaymentMethod: testsupport.Method, Currency: "UGX", Reference: "R", Account: "1"},
		}
		for name, req := range tests {
			if err := payment.ValidatePaymentPayload(domain.ServiceCollection, req); err == nil {
				t.Errorf("payment.ValidatePaymentPayload(%s) = nil, want an error", name)
			}
		}
	})
}

func TestProviderRequestValidation(t *testing.T) {
	t.Run("a normalized account is what gets persisted", func(t *testing.T) {
		s := testsupport.New(t)
		s.RegisterValidator("normalizing", func(req *providers.PaymentRequest) error {
			if !strings.HasPrefix(req.Account, "0") {
				return errors.New("acme: account must be a local mobile number")
			}
			req.Account, req.Scheme = "256"+strings.TrimPrefix(req.Account, "0"), "mtn"
			return nil
		})
		account := s.ProviderFor(t, "normalizing", testsupport.DummyConfig(nil), "UG")
		s.Route(t, account.ID, 1)
		app, _, _ := s.App(t)
		created := testsupport.Must(s.Payments.Create(
			context.Background(),
			app.ID,
			domain.ServiceCollection,
			"idem-normalize",
			testsupport.PaymentRequest("ORDER-NORM", "UG", "0770000000"),
		))
		var tx domain.Transaction
		testsupport.NoError(s.DB.First(&tx, "id = ?", created.TransactionID).Error)
		if tx.CustomerAccount != "256770000000" {
			t.Errorf("CustomerAccount = %q, want the account the provider normalized to", tx.CustomerAccount)
		}
	})

	t.Run("a rejected request leaves no transaction", func(t *testing.T) {
		s := testsupport.New(t)
		s.RegisterValidator("rejecting", func(*providers.PaymentRequest) error {
			return errors.New("acme: account is not a valid IBAN")
		})
		account := s.ProviderFor(t, "rejecting", testsupport.DummyConfig(nil), "UG")
		s.Route(t, account.ID, 1)
		app, _, _ := s.App(t)
		_, err := s.Payments.Create(
			context.Background(),
			app.ID,
			domain.ServiceCollection,
			"idem-reject",
			testsupport.PaymentRequest("ORDER-REJECT", "UG", "not-an-iban"),
		)
		if err == nil || !strings.Contains(err.Error(), "provider rejected the payment request") {
			t.Fatalf("Create() error = %v, want a provider rejection", err)
		}
		var count int64
		testsupport.NoError(s.DB.Model(&domain.Transaction{}).Count(&count).Error)
		if count != 0 {
			t.Errorf("transactions = %d, want none for a rejected request", count)
		}
	})

	t.Run("a provider may not rewrite a field it does not own", func(t *testing.T) {
		s := testsupport.New(t)
		s.RegisterValidator("mutating", func(req *providers.PaymentRequest) error {
			req.Amount = 1
			return nil
		})
		account := s.ProviderFor(t, "mutating", testsupport.DummyConfig(nil), "UG")
		s.Route(t, account.ID, 1)
		app, _, _ := s.App(t)
		_, err := s.Payments.Create(
			context.Background(),
			app.ID,
			domain.ServiceCollection,
			"idem-mutate",
			testsupport.PaymentRequest("ORDER-MUTATE", "UG", "256770000000"),
		)
		if err == nil || !strings.Contains(err.Error(), "does not own") {
			t.Fatalf("Create() error = %v, want the mutation guard to reject the request", err)
		}
		var count int64
		testsupport.NoError(s.DB.Model(&domain.Transaction{}).Count(&count).Error)
		if count != 0 {
			t.Errorf("transactions = %d, want none for a guarded request", count)
		}
	})
}
