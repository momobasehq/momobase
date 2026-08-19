package services_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/payment"
	"github.com/momobasehq/momobase/internal/routing"
	"github.com/momobasehq/momobase/internal/services"
	"github.com/momobasehq/momobase/internal/testsupport"
	"github.com/momobasehq/momobase/internal/utils"
)

func TestCoreFlows(t *testing.T) {
	t.Run("credential rotation", func(t *testing.T) {
		s := testsupport.New(t)
		app, credential, secret := s.App(t)
		token := testsupport.Must(s.Auth.IssueClientToken(context.Background(), credential.ClientID, secret))
		rotated := testsupport.Must(s.Apps.RotateCredential(context.Background(), s.Actor, app.ID, credential.ID))
		if _, err := s.Auth.ValidateClientCredentials(context.Background(), credential.ClientID, secret); err == nil {
			t.Fatal("old secret accepted")
		}
		testsupport.Must(s.Auth.ValidateClientCredentials(context.Background(), credential.ClientID, rotated.ClientSecret))
		if _, err := s.Auth.AuthenticateBearer(context.Background(), token.AccessToken); err == nil {
			t.Fatal("old access token accepted")
		}
	})
	t.Run("idempotency", func(t *testing.T) {
		s := testsupport.New(t)
		account := s.Provider(t, testsupport.DummyConfig(nil), "UG")
		s.Route(t, account.ID, 1)
		app, _, _ := s.App(t)
		req := &payment.CreatePaymentRequest{
			PaymentMethod: testsupport.Method,
			Amount:        50000,
			Currency:      "UGX",
			Country:       "UG",
			Reference:     "ORDER-1",
			Account:       "256770000000",
			Customer:      &payment.PartyPayload{Name: "Test Customer"},
		}
		first := testsupport.Must(s.Payments.Create(context.Background(), app.ID, domain.ServiceCollection, "idem", req))
		second := testsupport.Must(s.Payments.Create(context.Background(), app.ID, domain.ServiceCollection, "idem", req))
		if first.TransactionID != second.TransactionID {
			t.Fatal("idempotent request created a second transaction")
		}
		var attempts int64
		testsupport.NoError(s.DB.Model(&domain.TransactionAttempt{}).Where("transaction_id = ?", first.TransactionID).Count(&attempts).Error)
		if attempts != 1 {
			t.Fatalf("attempts=%d", attempts)
		}
	})
	t.Run("reconciliation settles a processing payment", func(t *testing.T) {
		s := testsupport.New(t)
		account := s.Provider(t, testsupport.DummyConfig(map[string]any{"settle_after": 2}), "UG")
		s.Route(t, account.ID, 1)
		app, _, _ := s.App(t)
		created := testsupport.Must(
			s.Payments.Create(context.Background(), app.ID, domain.ServiceCollection, "idem-recon", &payment.CreatePaymentRequest{
				PaymentMethod: testsupport.Method,
				Amount:        1500,
				Currency:      "UGX",
				Country:       "UG",
				Reference:     "ORDER-RECON",
				Account:       "256770000000",
				Customer:      &payment.PartyPayload{Name: "Test Customer"},
			}),
		)
		if created.Status != domain.TxProcessing {
			t.Fatalf("Create() status = %q, want %q", created.Status, domain.TxProcessing)
		}

		log := slog.New(slog.NewTextHandler(io.Discard, nil))
		recon := services.NewReconciliationService(s.DB, s.Runtime, services.NewWebhookService(s.DB, s.Runtime), log)
		// The orchestrator schedules the next attempt a minute out, so each pass
		// clears the backoff to make the run deterministic.
		for pass := range 2 {
			testsupport.NoError(s.DB.Model(&domain.Transaction{}).Where("id = ?", created.TransactionID).Update("next_reconcile_at", nil).Error)
			testsupport.NoError(recon.RunOnce(context.Background(), 10))
			var tx domain.Transaction
			testsupport.NoError(s.DB.First(&tx, "id = ?", created.TransactionID).Error)
			want := domain.TxProcessing
			if pass == 1 {
				want = domain.TxSucceeded
			}
			if tx.Status != want {
				t.Fatalf("status after pass %d = %q, want %q", pass+1, tx.Status, want)
			}
		}

		var attempt domain.TransactionAttempt
		testsupport.NoError(s.DB.First(&attempt, "transaction_id = ?", created.TransactionID).Error)
		if attempt.Status != domain.TxSucceeded || attempt.CompletedAt == nil {
			t.Fatalf("attempt = %q completed=%v, want a completed successful attempt", attempt.Status, attempt.CompletedAt)
		}
	})
	t.Run("country routing and health fallback", func(t *testing.T) {
		s := testsupport.New(t)
		primary, backup, kenya :=
			s.Provider(t, testsupport.DummyConfig(nil), "UG", "RW"),
			s.Provider(t, testsupport.DummyConfig(nil), "UG"),
			s.Provider(t, testsupport.DummyConfig(nil), "KE")
		s.Route(t, primary.ID, 1)
		s.Route(t, backup.ID, 2)
		s.Route(t, kenya.ID, 0)
		selected := testsupport.Must(s.Routing.SelectProvider(context.Background(), domain.ServiceCollection, testsupport.Method, "UG"))
		if selected.Account.ID != primary.ID {
			t.Fatal("highest-priority provider supporting UG was not selected")
		}
		testsupport.NoError(s.DB.Save(&domain.ProviderHealthSnapshot{
			ProviderAccountID: primary.ID,
			Status:            domain.ProviderDown,
			CircuitState:      domain.CircuitOpen,
		}).Error)
		selected = testsupport.Must(s.Routing.SelectProvider(context.Background(), domain.ServiceCollection, testsupport.Method, "UG"))
		if selected.Account.ID != backup.ID {
			t.Fatal("healthy UG backup was not selected")
		}
		if _, err := s.Routing.SelectProvider(
			context.Background(),
			domain.ServiceCollection,
			testsupport.Method,
			"TZ",
		); !errors.Is(err, routing.ErrNoRouteAvailable) {
			t.Fatalf("unexpected fallback for unsupported country: %v", err)
		}
	})
	t.Run("failed reload keeps runtime", func(t *testing.T) {
		s := testsupport.New(t)
		account := s.Provider(t, testsupport.DummyConfig(nil), "UG")
		before, _ := s.Runtime.Get(account.ID)
		if err := s.ProviderAdmin.UpdateConfig(
			context.Background(),
			s.Actor,
			account.ID,
			testsupport.DummyConfig(map[string]any{"fail_init": true}),
		); err == nil {
			t.Fatal("bad config accepted")
		}
		after, ok := s.Runtime.Get(account.ID)
		if !ok || after.ConfigVersion != before.ConfigVersion {
			t.Fatal("failed reload replaced the healthy runtime")
		}
	})
}

func TestCountryNormalization(t *testing.T) {
	countries, err := utils.NormalizeProviderCountries([]string{" ug ", "RW", "UG"})
	if err != nil || len(countries) != 2 || countries[0] != "UG" || countries[1] != "RW" {
		t.Fatalf("countries=%v err=%v", countries, err)
	}
	if countries, err = utils.NormalizeProviderCountries(nil); err != nil || countries != nil {
		t.Fatalf("utils.NormalizeProviderCountries(nil) = %v, %v, want an unrestricted account", countries, err)
	}
	if country, err := utils.NormalizeOptionalCountry("  "); err != nil || country != "" {
		t.Fatalf("utils.NormalizeOptionalCountry(blank) = %q, %v, want an empty country", country, err)
	}
	if country, err := utils.NormalizeOptionalCountry(" ug "); err != nil || country != "UG" {
		t.Fatalf("utils.NormalizeOptionalCountry(%q) = %q, %v", " ug ", country, err)
	}

	// language.ParseRegion is deliberately not trusted on its own: it accepts the
	// reserved and grouping regions, and rewrites alpha-3 input into alpha-2 rather
	// than rejecting it, so "USA" would otherwise be stored as "US".
	for _, country := range []string{"XX", "ZZ", "QO", "EU", "419", "USA", "U", "UGX"} {
		if got, err := utils.NormalizeTransactionCountry(country); err == nil {
			t.Errorf("utils.NormalizeTransactionCountry(%q) = %q, want an error", country, got)
		}
	}
	if _, err := utils.NormalizeTransactionCountry(""); err == nil {
		t.Error("utils.NormalizeTransactionCountry(\"\") = nil, want an error for a required country")
	}
	// UK is exceptionally reserved for GB rather than assigned, and ParseRegion
	// resolves it. Storing the caller's own input keeps it out of the canonical
	// rewrite path, so what round-trips is exactly what was sent.
	if country, err := utils.NormalizeTransactionCountry("uk"); err != nil || country != "UK" {
		t.Errorf("utils.NormalizeTransactionCountry(%q) = %q, %v, want the input uppercased", "uk", country, err)
	}
}

func TestServiceQueriesHonorCanceledContext(t *testing.T) {
	s := testsupport.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Apps.GetApp(ctx, "missing"); !errors.Is(err, context.Canceled) {
		t.Fatalf("GetApp() error = %v, want context canceled", err)
	}
}
