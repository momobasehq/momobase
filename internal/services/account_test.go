package services

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/providers"
	"github.com/momobasehq/momobase/providers/dummy"
)

// accountValidator wraps the dummy adapter with the optional RequestValidator hook,
// so a test can observe what the engine hands a provider and what it does with the
// request the provider hands back.
type accountValidator struct {
	providers.PaymentProvider
	validate func(*providers.PaymentRequest) error
}

func (p *accountValidator) ValidateRequest(_ context.Context, req *providers.PaymentRequest) error {
	return p.validate(req)
}

// registerValidator installs a validating adapter under a provider code. Each test
// gets its own stack, and therefore its own registry, so the closure cannot leak
// into another test.
func (s *testStack) registerValidator(code string, validate func(*providers.PaymentRequest) error) {
	s.registry.Register(code, func(log *slog.Logger) providers.PaymentProvider {
		return &accountValidator{PaymentProvider: dummy.New(log), validate: validate}
	})
}

func accountRequest(reference, country, account string) *CreatePaymentRequest {
	return &CreatePaymentRequest{
		PaymentMethod: testMethod,
		Amount:        1500,
		Currency:      "UGX",
		Country:       country,
		Reference:     reference,
		Account:       account,
	}
}

func TestValidatePaymentPayload(t *testing.T) {
	t.Run("normalizes the method, currency, country, and scheme", func(t *testing.T) {
		req := &CreatePaymentRequest{
			PaymentMethod: "  MOMO ",
			Amount:        100,
			Currency:      " ugx ",
			Country:       " ug ",
			Reference:     "ORDER-1",
			Account:       "  256770000000 ", Scheme: " MTN ",
			Customer: &PartyPayload{Name: "  Ada  "},
		}
		if err := ValidatePaymentPayload(domain.ServiceCollection, req); err != nil {
			t.Fatalf("ValidatePaymentPayload() error = %v", err)
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
		if err := ValidatePaymentPayload(domain.ServiceCollection, req); err != nil {
			t.Fatalf("ValidatePaymentPayload() error = %v, want a rail-agnostic account accepted", err)
		}
		if req.Country != "" {
			t.Errorf("Country = %q, want it left empty", req.Country)
		}
	})

	t.Run("rejects a malformed request", func(t *testing.T) {
		tests := map[string]*CreatePaymentRequest{
			"missing account":  {PaymentMethod: testMethod, Amount: 1, Currency: "UGX", Reference: "R"},
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
				PaymentMethod: testMethod,
				Amount:        1,
				Currency:      "UGX",
				Reference:     "R",
				Account:       "1", Scheme: "mtn/ug",
			},
			"zero amount": {PaymentMethod: testMethod, Currency: "UGX", Reference: "R", Account: "1"},
		}
		for name, req := range tests {
			if err := ValidatePaymentPayload(domain.ServiceCollection, req); err == nil {
				t.Errorf("ValidatePaymentPayload(%s) = nil, want an error", name)
			}
		}
	})
}

func TestProviderRequestValidation(t *testing.T) {
	t.Run("a normalized account is what gets persisted", func(t *testing.T) {
		s := stack(t)
		s.registerValidator("normalizing", func(req *providers.PaymentRequest) error {
			if !strings.HasPrefix(req.Account, "0") {
				return errors.New("acme: account must be a local mobile number")
			}
			req.Account, req.Scheme = "256"+strings.TrimPrefix(req.Account, "0"), "mtn"
			return nil
		})
		account := s.providerFor(t, "normalizing", dummyConfig(nil), "UG")
		s.route(t, account.ID, 1)
		app, _, _ := s.app(t)
		created := must(s.payments.Create(
			context.Background(),
			app.ID,
			domain.ServiceCollection,
			"idem-normalize",
			accountRequest("ORDER-NORM", "UG", "0770000000"),
		))
		var tx domain.Transaction
		noError(s.db.First(&tx, "id = ?", created.TransactionID).Error)
		if tx.CustomerAccount != "256770000000" {
			t.Errorf("CustomerAccount = %q, want the account the provider normalized to", tx.CustomerAccount)
		}
	})

	t.Run("a rejected request leaves no transaction", func(t *testing.T) {
		s := stack(t)
		s.registerValidator("rejecting", func(*providers.PaymentRequest) error {
			return errors.New("acme: account is not a valid IBAN")
		})
		account := s.providerFor(t, "rejecting", dummyConfig(nil), "UG")
		s.route(t, account.ID, 1)
		app, _, _ := s.app(t)
		_, err := s.payments.Create(
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
		noError(s.db.Model(&domain.Transaction{}).Count(&count).Error)
		if count != 0 {
			t.Errorf("transactions = %d, want none for a rejected request", count)
		}
	})

	t.Run("a provider may not rewrite a field it does not own", func(t *testing.T) {
		s := stack(t)
		s.registerValidator("mutating", func(req *providers.PaymentRequest) error {
			req.Amount = 1
			return nil
		})
		account := s.providerFor(t, "mutating", dummyConfig(nil), "UG")
		s.route(t, account.ID, 1)
		app, _, _ := s.app(t)
		_, err := s.payments.Create(
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
		noError(s.db.Model(&domain.Transaction{}).Count(&count).Error)
		if count != 0 {
			t.Errorf("transactions = %d, want none for a guarded request", count)
		}
	})

	t.Run("an emptied account is rejected", func(t *testing.T) {
		if err := guardValidatedRequest(
			providers.PaymentRequest{Account: "256770000000"},
			&providers.PaymentRequest{},
		); err == nil {
			t.Fatal("guardValidatedRequest() = nil, want an error for an emptied account")
		}
	})
}

func TestCountryOptionalRouting(t *testing.T) {
	t.Run("an unrestricted provider serves a payment with no country", func(t *testing.T) {
		s := stack(t)
		account := s.provider(t, dummyConfig(nil))
		s.route(t, account.ID, 1)
		app, _, _ := s.app(t)
		created := must(s.payments.Create(
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
		noError(s.db.First(&tx, "id = ?", created.TransactionID).Error)
		if tx.Country != "" || tx.CustomerAccount != "acct_4f9c" {
			t.Errorf("transaction = %+v, want no country and the opaque account", tx)
		}
	})

	t.Run("a country-scoped provider does not serve a payment with no country", func(t *testing.T) {
		s := stack(t)
		account := s.provider(t, dummyConfig(nil), "UG")
		s.route(t, account.ID, 1)
		if _, err := s.routing.SelectProvider(
			context.Background(),
			domain.ServiceCollection,
			testMethod,
			"",
		); !errors.Is(err, ErrNoRouteAvailable) {
			t.Fatalf("SelectProvider() error = %v, want %v", err, ErrNoRouteAvailable)
		}
	})

	t.Run("eligibility", func(t *testing.T) {
		if !countryEligible(nil, "UG") || !countryEligible(nil, "") {
			t.Error("countryEligible() = false, want an account with no countries to be unrestricted")
		}
		if !countryEligible([]string{"UG"}, "UG") {
			t.Error("countryEligible(UG, UG) = false, want true")
		}
		if countryEligible([]string{"UG"}, "RW") || countryEligible([]string{"UG"}, "") {
			t.Error("countryEligible() = true, want a declared country to be required")
		}
	})
}

func TestWebhookAccountMatching(t *testing.T) {
	tx := &domain.Transaction{
		Amount:          1500,
		Currency:        "UGX",
		Country:         "UG",
		Reference:       "ORDER-1",
		CustomerAccount: "256770000000",
	}
	matching := &VerifiedWebhook{ProviderWebhookEvent: providers.ProviderWebhookEvent{Account: "256770000000"}}
	if err := validateWebhook(matching, tx); err != nil {
		t.Fatalf("validateWebhook() error = %v, want the recorded account to match", err)
	}
	absent := &VerifiedWebhook{}
	if err := validateWebhook(absent, tx); err != nil {
		t.Fatalf("validateWebhook() error = %v, want an event without an account to skip the check", err)
	}
	// A provider that reports an unnormalized account no longer matches: the engine
	// compares exactly, so normalization is the provider's to keep consistent.
	other := &VerifiedWebhook{ProviderWebhookEvent: providers.ProviderWebhookEvent{Account: "0770000000"}}
	if err := validateWebhook(other, tx); err == nil {
		t.Fatal("validateWebhook() = nil, want a mismatched account rejected")
	}
}

func TestAvailablePaymentMethods(t *testing.T) {
	t.Run("lists only what would actually route", func(t *testing.T) {
		s := stack(t)
		account := s.provider(t, dummyConfig(nil))
		s.route(t, account.ID, 1)

		methods := must(s.routing.AvailablePaymentMethods(context.Background(), "", ""))
		if len(methods) != 1 ||
			methods[0].PaymentMethod != testMethod ||
			methods[0].ServiceType != domain.ServiceCollection {
			t.Fatalf("AvailablePaymentMethods() = %+v, want the one routable collection method", methods)
		}
	})

	t.Run("a country-scoped provider is not offered to a countryless client", func(t *testing.T) {
		s := stack(t)
		account := s.provider(t, dummyConfig(nil), "UG")
		s.route(t, account.ID, 1)

		if methods := must(s.routing.AvailablePaymentMethods(context.Background(), "", "")); len(methods) != 0 {
			t.Errorf("AvailablePaymentMethods(no country) = %+v, want none", methods)
		}
		if methods := must(s.routing.AvailablePaymentMethods(context.Background(), "", "UG")); len(methods) != 1 {
			t.Errorf("AvailablePaymentMethods(UG) = %+v, want the scoped method", methods)
		}
	})

	// The listing and SelectProvider must agree: anything offered has to be payable,
	// or a checkout screen shows a method that 503s the moment it is used.
	t.Run("agrees with SelectProvider", func(t *testing.T) {
		s := stack(t)
		account := s.provider(t, dummyConfig(nil))
		s.route(t, account.ID, 1)

		for _, method := range must(s.routing.AvailablePaymentMethods(context.Background(), "", "")) {
			if _, err := s.routing.SelectProvider(
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
		s := stack(t)
		if _, err := s.routing.AvailablePaymentMethods(context.Background(), "refund", ""); err == nil {
			t.Error("AvailablePaymentMethods(refund) = nil, want an error")
		}
		if _, err := s.routing.AvailablePaymentMethods(context.Background(), "", "ZZ"); err == nil {
			t.Error("AvailablePaymentMethods(ZZ) = nil, want an error")
		}
	})

	t.Run("an inactive provider account removes its method", func(t *testing.T) {
		s := stack(t)
		account := s.provider(t, dummyConfig(nil))
		s.route(t, account.ID, 1)
		noError(s.providerAdmin.Deactivate(context.Background(), s.actor, account.ID))

		if methods := must(s.routing.AvailablePaymentMethods(context.Background(), "", "")); len(methods) != 0 {
			t.Errorf("AvailablePaymentMethods() = %+v, want none once the account is inactive", methods)
		}
	})
}
