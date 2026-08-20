package routing_test

import (
	"context"
	"errors"
	"testing"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/service/routing"
	"github.com/momobasehq/momobase/internal/testsupport"
)

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
			testsupport.PaymentRequest("ORDER-GLOBAL", "", "acct_4f9c"),
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
