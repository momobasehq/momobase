package routing_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/service/routing"
	"github.com/momobasehq/momobase/internal/testsupport"
	"github.com/momobasehq/momobase/providers"
)

func TestRouteCreationRequiresProviderCapability(t *testing.T) {
	s := testsupport.New(t)
	account := s.Provider(t, testsupport.DummyConfig(nil))
	runtime, ok := s.Runtime.Get(account.ID)
	if !ok {
		t.Fatal("active provider runtime is missing")
	}
	runtime.Capabilities = []providers.Capability{{
		ServiceType:   domain.ServiceCollection,
		PaymentMethod: providers.PaymentMethodMomo,
	}}

	_, err := s.Routes.Create(
		context.Background(),
		s.Actor,
		domain.ServiceCollection,
		providers.PaymentMethodCard,
		account.ID,
		1,
		true,
	)
	if err == nil || !strings.Contains(err.Error(), "does not support collection/card") {
		t.Fatalf("Create() error = %v, want unsupported capability", err)
	}
}

func TestProviderLocationRouting(t *testing.T) {
	s := testsupport.New(t)
	account := s.Provider(t, testsupport.DummyConfig(nil), "UG")
	s.Route(t, account.ID, 1)

	for _, location := range []struct{ country, currency string }{
		{country: "KE", currency: "UGX"},
		{country: "UG", currency: "USD"},
	} {
		if _, err := s.Routing.SelectProvider(
			context.Background(),
			domain.ServiceCollection,
			testsupport.Method,
			location.country,
			location.currency,
		); !errors.Is(err, routing.ErrNoRouteAvailable) {
			t.Fatalf("SelectProvider(%+v) error = %v, want %v", location, err, routing.ErrNoRouteAvailable)
		}
	}
}

func TestAvailablePaymentMethods(t *testing.T) {
	t.Run("lists only what would actually route", func(t *testing.T) {
		s := testsupport.New(t)
		account := s.Provider(t, testsupport.DummyConfig(nil))
		s.Route(t, account.ID, 1)

		methods := testsupport.Must(s.Routing.AvailablePaymentMethods(context.Background(), "", "UG", "UGX"))
		if len(methods) != 1 ||
			methods[0].PaymentMethod != testsupport.Method ||
			methods[0].ServiceType != domain.ServiceCollection {
			t.Fatalf("AvailablePaymentMethods() = %+v, want the one routable collection method", methods)
		}
	})

	t.Run("a provider is offered only for its country and currency", func(t *testing.T) {
		s := testsupport.New(t)
		account := s.Provider(t, testsupport.DummyConfig(nil), "UG")
		s.Route(t, account.ID, 1)

		if methods := testsupport.Must(s.Routing.AvailablePaymentMethods(context.Background(), "", "KE", "UGX")); len(methods) != 0 {
			t.Errorf("AvailablePaymentMethods(KE, UGX) = %+v, want none", methods)
		}
		if methods := testsupport.Must(s.Routing.AvailablePaymentMethods(context.Background(), "", "UG", "UGX")); len(methods) != 1 {
			t.Errorf("AvailablePaymentMethods(UG, UGX) = %+v, want the scoped method", methods)
		}
	})

	// The listing and SelectProvider must agree: anything offered has to be payable,
	// or a checkout screen shows a method that 503s the moment it is used.
	t.Run("agrees with SelectProvider", func(t *testing.T) {
		s := testsupport.New(t)
		account := s.Provider(t, testsupport.DummyConfig(nil))
		s.Route(t, account.ID, 1)

		for _, method := range testsupport.Must(s.Routing.AvailablePaymentMethods(context.Background(), "", "UG", "UGX")) {
			if _, err := s.Routing.SelectProvider(
				context.Background(),
				method.ServiceType,
				method.PaymentMethod,
				"UG",
				"UGX",
			); err != nil {
				t.Errorf("SelectProvider(%+v) error = %v, want a listed method to be routable", method, err)
			}
		}
	})

	t.Run("rejects an unknown service and an invalid country", func(t *testing.T) {
		s := testsupport.New(t)
		if _, err := s.Routing.AvailablePaymentMethods(context.Background(), "refund", "UG", "UGX"); err == nil {
			t.Error("AvailablePaymentMethods(refund) = nil, want an error")
		}
		if _, err := s.Routing.AvailablePaymentMethods(context.Background(), "", "ZZ", "UGX"); err == nil {
			t.Error("AvailablePaymentMethods(ZZ) = nil, want an error")
		}
	})

	t.Run("an inactive provider account removes its method", func(t *testing.T) {
		s := testsupport.New(t)
		account := s.Provider(t, testsupport.DummyConfig(nil))
		s.Route(t, account.ID, 1)
		testsupport.NoError(s.ProviderAdmin.Deactivate(context.Background(), s.Actor, account.ID))

		if methods := testsupport.Must(s.Routing.AvailablePaymentMethods(context.Background(), "", "UG", "UGX")); len(methods) != 0 {
			t.Errorf("AvailablePaymentMethods() = %+v, want none once the account is inactive", methods)
		}
	})
}
