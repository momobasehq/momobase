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
// Momobase reads no environment variables and no configuration files. New uses
// [DefaultConfig] — a development baseline of plain values — unless a host supplies
// its own through [WithConfig]. Copy the default, change what differs, and pass it:
//
//	cfg := momobase.DefaultConfig()
//	cfg.App.Env = "production"
//	cfg.App.PublicURL = "https://payments.example.com"
//	cfg.DB = momobase.DatabaseConfig{Type: "postgres", Host: "db.internal", ...}
//
//	instance, err := momobase.New(
//		momobase.WithConfig(cfg),
//		momobase.WithProvider("acme_pay", acme.New),
//	)
//
// A host that configures from the environment, a file, or a secret manager reads
// it itself and assigns the fields, so the source of a value is the host's choice
// rather than this package's. [Config.Validate] rejects the default credentials
// and other unsafe settings when App.Env is staging or production.
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
