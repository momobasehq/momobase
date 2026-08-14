// Package momobase embeds the Momobase mobile-money orchestration server in a Go
// application and extends it with custom payment providers.
//
// New constructs an instance from configuration and the registered providers,
// and Run or Serve starts its HTTP server and background workers:
//
//	instance, err := momobase.New(
//		momobase.WithProvider("mtn_momo", mtn.New),
//		momobase.WithProvider("acme_pay", acme.New),
//	)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer instance.Close()
//	log.Fatal(instance.Run())
//
// A provider is any type implementing [PaymentProvider], registered under a
// provider code with [WithProvider]. Accounts for that code are then created,
// configured, and activated through the Admin API, and the configuration
// recorded for an account is passed to the provider's Init method as a
// [ProviderConfig].
//
// Momobase registers no providers on its own: a build carries exactly the
// providers it asks for, and New reports an error when none are registered. The
// adapters shipped with Momobase live in providers/mtn and providers/airtel and
// are registered like any other.
package momobase
