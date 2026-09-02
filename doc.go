// Package momobase embeds the Momobase payment orchestration server in a Go
// application and extends it with custom payment providers.
//
// New constructs an instance from configuration and the registered providers,
// and Run or Serve starts its HTTP server and background workers:
//
//	instance, err := momobase.New(
//		momobase.WithProvider("acme_pay", acme.New),
//	)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer instance.Close()
//	log.Fatal(instance.Run())
//
// Provider contracts and helpers live in the providers package. Register a
// providers.PaymentProvider factory under a provider code with [WithProvider].
// Accounts for that code are then created, configured, and activated through
// the Admin API.
//
// Momobase registers no providers on its own: a build carries exactly the
// providers it asks for, and New reports an error when none are registered.
// providers/dummy is the included reference adapter and moves no money.
//
// @title Momobase API
// @version 1.0
// @description Embeddable payment orchestration API for application payments and administrative operations.
// @BasePath /
// @schemes http https
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Enter a bearer token using the format: Bearer {token}
package momobase
