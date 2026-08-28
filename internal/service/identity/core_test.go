package identity_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/momobasehq/momobase/hooks"
	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/dto"
	"github.com/momobasehq/momobase/internal/service/reconciliation"
	"github.com/momobasehq/momobase/internal/service/routing"
	"github.com/momobasehq/momobase/internal/service/webhook"
	"github.com/momobasehq/momobase/internal/testsupport"
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
		req := &dto.CreatePayment{
			PaymentMethod: testsupport.Method,
			Amount:        50000,
			Currency:      "UGX",
			Country:       "UG",
			Reference:     "ORDER-1",
			Account:       "256770000000",
			Customer:      &dto.Party{Name: "Test Customer"},
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
		changes := []hooks.TransactionChangedEvent{}
		s.Hooks.OnTransactionChanged().Bind(func(_ context.Context, event hooks.TransactionChangedEvent) error {
			changes = append(changes, event)
			return nil
		})
		created := testsupport.Must(
			s.Payments.Create(context.Background(), app.ID, domain.ServiceCollection, "idem-recon", &dto.CreatePayment{
				PaymentMethod: testsupport.Method,
				Amount:        1500,
				Currency:      "UGX",
				Country:       "UG",
				Reference:     "ORDER-RECON",
				Account:       "256770000000",
				Customer:      &dto.Party{Name: "Test Customer"},
			}),
		)
		if created.Status != domain.TxProcessing {
			t.Fatalf("Create() status = %q, want %q", created.Status, domain.TxProcessing)
		}

		log := slog.New(slog.NewTextHandler(io.Discard, nil))
		recon := reconciliation.New(reconciliation.Deps{
			Repos:   s.Repos,
			Runtime: s.Runtime,
			Webhook: webhook.New(s.Repos, s.Runtime, s.Hooks),
			Logger:  log,
			Hooks:   s.Hooks,
		})
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
		isExpectedSequence := len(changes) == 2 &&
			changes[0].Source == hooks.TransactionSourceRequest &&
			changes[1].Source == hooks.TransactionSourceReconciliation &&
			changes[1].Status == domain.TxSucceeded
		if !isExpectedSequence {
			t.Errorf("transaction change events = %+v, want request then successful reconciliation", changes)
		}
	})
	t.Run("country routing and health fallback", func(t *testing.T) {
		s := testsupport.New(t)
		primary, backup, kenya :=
			s.Provider(t, testsupport.DummyConfig(nil), "UG"),
			s.Provider(t, testsupport.DummyConfig(nil), "UG"),
			s.Provider(t, testsupport.DummyConfig(nil), "KE")
		s.Route(t, primary.ID, 1)
		s.Route(t, backup.ID, 2)
		s.Route(t, kenya.ID, 0)
		selected := testsupport.Must(s.Routing.SelectProvider(context.Background(), domain.ServiceCollection, testsupport.Method, "UG", "UGX"))
		if selected.Account.ID != primary.ID {
			t.Fatal("highest-priority provider supporting UG was not selected")
		}
		testsupport.NoError(s.DB.Save(&domain.ProviderHealthSnapshot{
			ProviderAccountID: primary.ID,
			Status:            domain.ProviderDown,
			CircuitState:      domain.CircuitOpen,
		}).Error)
		selected = testsupport.Must(s.Routing.SelectProvider(context.Background(), domain.ServiceCollection, testsupport.Method, "UG", "UGX"))
		if selected.Account.ID != backup.ID {
			t.Fatal("healthy UG backup was not selected")
		}
		if _, err := s.Routing.SelectProvider(
			context.Background(),
			domain.ServiceCollection,
			testsupport.Method,
			"TZ",
			"UGX",
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

func TestServiceQueriesHonorCanceledContext(t *testing.T) {
	s := testsupport.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Apps.GetApp(ctx, "missing"); !errors.Is(err, context.Canceled) {
		t.Fatalf("GetApp() error = %v, want context canceled", err)
	}
}
