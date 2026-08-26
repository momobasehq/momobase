package payment_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/dto"
	"github.com/momobasehq/momobase/internal/service/identity"
	providerService "github.com/momobasehq/momobase/internal/service/provider"
	"github.com/momobasehq/momobase/internal/testsupport"
	"github.com/momobasehq/momobase/providers"
)

func TestCreatePaymentPayloadRules(t *testing.T) {
	t.Run("normalizes the method, currency, country, and scheme", func(t *testing.T) {
		req := &dto.CreatePayment{
			PaymentMethod: "  MOMO ",
			Amount:        100,
			Currency:      " ugx ",
			Country:       " ug ",
			Reference:     "ORDER-1",
			Account:       "  256770000000 ", Scheme: " MTN ",
			Customer: &dto.Party{Name: "  Ada  "},
		}
		if err := dto.Check(req); err != nil {
			t.Fatalf("dto.Check() error = %v", err)
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

	t.Run("requires a country", func(t *testing.T) {
		req := testsupport.PaymentRequest("ORDER-2", "", "GB33BUKB20201555555555")
		if err := dto.Check(req); err == nil {
			t.Fatal("dto.Check() = nil, want a missing-country error")
		}
	})

	t.Run("rejects a malformed request", func(t *testing.T) {
		tests := map[string]*dto.CreatePayment{
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
			if err := dto.Check(req); err == nil {
				t.Errorf("dto.Check(%s) = nil, want an error", name)
			}
		}
	})
}

func TestCreateSnapshotsFeesOnce(t *testing.T) {
	s := testsupport.New(t)
	account := s.Provider(t, testsupport.DummyConfig(nil), "UG")
	s.Route(t, account.ID, 1)
	app, _, _ := s.App(t)

	appCharges := domain.ChargeSchedule{
		Collection:   domain.ChargeRule{Type: domain.ChargePercentage, Value: 1_000},
		Disbursement: domain.ChargeRule{Type: domain.ChargeFlat},
	}
	providerCharges := domain.ChargeSchedule{
		Collection:   domain.ChargeRule{Type: domain.ChargeFlat, Value: 75},
		Disbursement: domain.ChargeRule{Type: domain.ChargeFlat},
	}
	_, err := s.Apps.UpdateApp(context.Background(), s.Actor, app.ID, identity.UpdateAppInput{
		Description: app.Description,
		Charges:     &appCharges,
	})
	testsupport.NoError(err)
	testsupport.NoError(s.ProviderAdmin.UpdateSettings(
		context.Background(),
		s.Actor,
		account.ID,
		providerService.AccountSettings{Country: "UG", Currency: "UGX", Charges: providerCharges},
	))

	req := testsupport.PaymentRequest("ORDER-FEES", "UG", "256770000000")
	created := testsupport.Must(s.Payments.Create(
		context.Background(),
		app.ID,
		domain.ServiceCollection,
		"idem-fees",
		req,
	))
	if created.PlatformFee != 150 {
		t.Fatalf("PlatformFee = %d, want 150", created.PlatformFee)
	}
	var tx domain.Transaction
	testsupport.NoError(s.DB.First(&tx, "id = ?", created.TransactionID).Error)
	if tx.Amount != 1_500 || tx.ProviderFee != 75 || tx.PlatformFee != 150 {
		t.Fatalf("transaction amount/fees = %d/%d/%d, want 1500/75/150", tx.Amount, tx.ProviderFee, tx.PlatformFee)
	}

	zero := domain.ChargeSchedule{}
	zero.Normalize()
	_, err = s.Apps.UpdateApp(context.Background(), s.Actor, app.ID, identity.UpdateAppInput{
		Description: app.Description,
		Charges:     &zero,
	})
	testsupport.NoError(err)
	testsupport.NoError(s.ProviderAdmin.UpdateSettings(
		context.Background(),
		s.Actor,
		account.ID,
		providerService.AccountSettings{Country: "UG", Currency: "UGX", Charges: zero},
	))
	replayed := testsupport.Must(s.Payments.Create(
		context.Background(),
		app.ID,
		domain.ServiceCollection,
		"idem-fees",
		testsupport.PaymentRequest("ORDER-FEES", "UG", "256770000000"),
	))
	if replayed.PlatformFee != 150 {
		t.Errorf("replay PlatformFee = %d, want original 150", replayed.PlatformFee)
	}
	testsupport.NoError(s.DB.First(&tx, "id = ?", created.TransactionID).Error)
	if tx.ProviderFee != 75 || tx.PlatformFee != 150 {
		t.Errorf("stored replay fees = %d/%d, want 75/150", tx.ProviderFee, tx.PlatformFee)
	}
}

func TestCreateRejectsCurrencyOutsideApp(t *testing.T) {
	s := testsupport.New(t)
	account := s.Provider(t, testsupport.DummyConfig(nil), "UG")
	s.Route(t, account.ID, 1)
	app, _, _ := s.App(t)
	req := testsupport.PaymentRequest("ORDER-USD", "UG", "256770000000")
	req.Currency = "USD"

	_, err := s.Payments.Create(
		context.Background(),
		app.ID,
		domain.ServiceCollection,
		"idem-usd",
		req,
	)
	if err == nil || !strings.Contains(err.Error(), "must match app currency UGX") {
		t.Fatalf("Create() error = %v, want app currency rejection", err)
	}
	var count int64
	testsupport.NoError(s.DB.Model(&domain.Transaction{}).Count(&count).Error)
	if count != 0 {
		t.Errorf("transactions = %d, want none for a currency mismatch", count)
	}
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

// TestIdempotencyIsDecidedAfterNormalizationAndBeforeTheProvider pins the ordering the
// whole create path depends on. The request hash is taken over the normalized payload
// and before the selected provider's RequestValidator runs, which decides two things
// that would otherwise be silent: two spellings of one request are the same request,
// and a provider rewriting the account afterwards cannot change the identity of a
// request that was already made.
func TestIdempotencyIsDecidedAfterNormalizationAndBeforeTheProvider(t *testing.T) {
	t.Run("a differently spelled body replays rather than erroring", func(t *testing.T) {
		s := testsupport.New(t)
		// The validator rewrites the account, so if the hash were taken after it ran,
		// the second call would hash a different payload and be refused as a reuse.
		s.RegisterValidator("normalizing", func(req *providers.PaymentRequest) error {
			req.Account, req.Scheme = "256"+strings.TrimPrefix(req.Account, "0"), "mtn"
			return nil
		})
		account := s.ProviderFor(t, "normalizing", testsupport.DummyConfig(nil), "UG")
		s.Route(t, account.ID, 1)
		app, _, _ := s.App(t)

		spelled := testsupport.PaymentRequest("ORDER-IDEM", " ug ", "0770000000")
		spelled.Currency, spelled.PaymentMethod = " ugx ", strings.ToUpper(testsupport.Method)
		first := testsupport.Must(s.Payments.Create(
			context.Background(), app.ID, domain.ServiceCollection, "idem-spelling", spelled,
		))

		// Same request, written the way it normalizes to.
		canonical := testsupport.PaymentRequest("ORDER-IDEM", "UG", "0770000000")
		second, err := s.Payments.Create(
			context.Background(), app.ID, domain.ServiceCollection, "idem-spelling", canonical,
		)
		if err != nil {
			t.Fatalf("Create(same request, other spelling) error = %v, want a replay", err)
		}
		if second.TransactionID != first.TransactionID {
			t.Errorf("replay transaction = %q, want the original %q", second.TransactionID, first.TransactionID)
		}
	})

	t.Run("a genuinely different body is refused", func(t *testing.T) {
		s := testsupport.New(t)
		account := s.Provider(t, testsupport.DummyConfig(nil), "UG")
		s.Route(t, account.ID, 1)
		app, _, _ := s.App(t)

		testsupport.Must(s.Payments.Create(
			context.Background(), app.ID, domain.ServiceCollection, "idem-differs",
			testsupport.PaymentRequest("ORDER-A", "UG", "256770000000"),
		))
		_, err := s.Payments.Create(
			context.Background(), app.ID, domain.ServiceCollection, "idem-differs",
			testsupport.PaymentRequest("ORDER-B", "UG", "256770000000"),
		)
		if err == nil || !strings.Contains(err.Error(), "different request") {
			t.Fatalf("Create(different body, same key) error = %v, want a reuse refusal", err)
		}
	})
}
