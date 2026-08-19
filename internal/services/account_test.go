package services_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/routing"
	"github.com/momobasehq/momobase/internal/services"
	"github.com/momobasehq/momobase/internal/testsupport"
	"github.com/momobasehq/momobase/providers"
)

func accountRequest(reference, country, account string) *services.CreatePaymentRequest {
	return &services.CreatePaymentRequest{
		PaymentMethod: testsupport.Method,
		Amount:        1500,
		Currency:      "UGX",
		Country:       country,
		Reference:     reference,
		Account:       account,
	}
}

func TestValidatePaymentPayload(t *testing.T) {
	t.Run("normalizes the method, currency, country, and scheme", func(t *testing.T) {
		req := &services.CreatePaymentRequest{
			PaymentMethod: "  MOMO ",
			Amount:        100,
			Currency:      " ugx ",
			Country:       " ug ",
			Reference:     "ORDER-1",
			Account:       "  256770000000 ", Scheme: " MTN ",
			Customer: &services.PartyPayload{Name: "  Ada  "},
		}
		if err := services.ValidatePaymentPayload(domain.ServiceCollection, req); err != nil {
			t.Fatalf("services.ValidatePaymentPayload() error = %v", err)
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
		req := accountRequest("ORDER-2", "", "GB33BUKB20201555555555")
		if err := services.ValidatePaymentPayload(domain.ServiceCollection, req); err != nil {
			t.Fatalf("services.ValidatePaymentPayload() error = %v, want a rail-agnostic account accepted", err)
		}
		if req.Country != "" {
			t.Errorf("Country = %q, want it left empty", req.Country)
		}
	})

	t.Run("rejects a malformed request", func(t *testing.T) {
		tests := map[string]*services.CreatePaymentRequest{
			"missing account":  {PaymentMethod: testsupport.Method, Amount: 1, Currency: "UGX", Reference: "R"},
			"blank account":    accountRequest("R", "", "   "),
			"control account":  accountRequest("R", "", "2567700\x0000"),
			"oversize account": accountRequest("R", "", strings.Repeat("9", 256)),
			"unknown country":  accountRequest("R", "XX", "256770000000"),
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
			if err := services.ValidatePaymentPayload(domain.ServiceCollection, req); err == nil {
				t.Errorf("services.ValidatePaymentPayload(%s) = nil, want an error", name)
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
			accountRequest("ORDER-NORM", "UG", "0770000000"),
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
			accountRequest("ORDER-REJECT", "UG", "not-an-iban"),
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
			accountRequest("ORDER-MUTATE", "UG", "256770000000"),
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

func TestCountryOptionalRouting(t *testing.T) {
	t.Run("an unrestricted provider serves a payment with no country", func(t *testing.T) {
		s := testsupport.New(t)
		account := s.Provider(t, testsupport.DummyConfig(nil))
		s.Route(t, account.ID, 1)
		app, _, _ := s.App(t)
		created := testsupport.Must(s.Payments.Create(
			context.Background(),
			app.ID,
			domain.ServiceCollection,
			"idem-global",
			accountRequest("ORDER-GLOBAL", "", "acct_4f9c"),
		))
		if created.Status != domain.TxSucceeded {
			t.Fatalf("Create() status = %q, want %q", created.Status, domain.TxSucceeded)
		}
		var tx domain.Transaction
		testsupport.NoError(s.DB.First(&tx, "id = ?", created.TransactionID).Error)
		if tx.Country != "" || tx.CustomerAccount != "acct_4f9c" {
			t.Errorf("transaction = %+v, want no country and the opaque account", tx)
		}
	})

	t.Run("a country-scoped provider does not serve a payment with no country", func(t *testing.T) {
		s := testsupport.New(t)
		account := s.Provider(t, testsupport.DummyConfig(nil), "UG")
		s.Route(t, account.ID, 1)
		if _, err := s.Routing.SelectProvider(
			context.Background(),
			domain.ServiceCollection,
			testsupport.Method,
			"",
		); !errors.Is(err, routing.ErrNoRouteAvailable) {
			t.Fatalf("SelectProvider() error = %v, want %v", err, routing.ErrNoRouteAvailable)
		}
	})
}

func TestAvailablePaymentMethods(t *testing.T) {
	t.Run("lists only what would actually route", func(t *testing.T) {
		s := testsupport.New(t)
		account := s.Provider(t, testsupport.DummyConfig(nil))
		s.Route(t, account.ID, 1)

		methods := testsupport.Must(s.Routing.AvailablePaymentMethods(context.Background(), "", ""))
		if len(methods) != 1 ||
			methods[0].PaymentMethod != testsupport.Method ||
			methods[0].ServiceType != domain.ServiceCollection {
			t.Fatalf("AvailablePaymentMethods() = %+v, want the one routable collection method", methods)
		}
	})

	t.Run("a country-scoped provider is not offered to a countryless client", func(t *testing.T) {
		s := testsupport.New(t)
		account := s.Provider(t, testsupport.DummyConfig(nil), "UG")
		s.Route(t, account.ID, 1)

		if methods := testsupport.Must(s.Routing.AvailablePaymentMethods(context.Background(), "", "")); len(methods) != 0 {
			t.Errorf("AvailablePaymentMethods(no country) = %+v, want none", methods)
		}
		if methods := testsupport.Must(s.Routing.AvailablePaymentMethods(context.Background(), "", "UG")); len(methods) != 1 {
			t.Errorf("AvailablePaymentMethods(UG) = %+v, want the scoped method", methods)
		}
	})

	// The listing and SelectProvider must agree: anything offered has to be payable,
	// or a checkout screen shows a method that 503s the moment it is used.
	t.Run("agrees with SelectProvider", func(t *testing.T) {
		s := testsupport.New(t)
		account := s.Provider(t, testsupport.DummyConfig(nil))
		s.Route(t, account.ID, 1)

		for _, method := range testsupport.Must(s.Routing.AvailablePaymentMethods(context.Background(), "", "")) {
			if _, err := s.Routing.SelectProvider(
				context.Background(),
				method.ServiceType,
				method.PaymentMethod,
				"",
			); err != nil {
				t.Errorf("SelectProvider(%+v) error = %v, want a listed method to be routable", method, err)
			}
		}
	})

	t.Run("rejects an unknown service and an invalid country", func(t *testing.T) {
		s := testsupport.New(t)
		if _, err := s.Routing.AvailablePaymentMethods(context.Background(), "refund", ""); err == nil {
			t.Error("AvailablePaymentMethods(refund) = nil, want an error")
		}
		if _, err := s.Routing.AvailablePaymentMethods(context.Background(), "", "ZZ"); err == nil {
			t.Error("AvailablePaymentMethods(ZZ) = nil, want an error")
		}
	})

	t.Run("an inactive provider account removes its method", func(t *testing.T) {
		s := testsupport.New(t)
		account := s.Provider(t, testsupport.DummyConfig(nil))
		s.Route(t, account.ID, 1)
		testsupport.NoError(s.ProviderAdmin.Deactivate(context.Background(), s.Actor, account.ID))

		if methods := testsupport.Must(s.Routing.AvailablePaymentMethods(context.Background(), "", "")); len(methods) != 0 {
			t.Errorf("AvailablePaymentMethods() = %+v, want none once the account is inactive", methods)
		}
	})
}
