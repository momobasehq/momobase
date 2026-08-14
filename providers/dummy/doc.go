// Package dummy provides a payment provider that simulates payments entirely in
// memory. It performs no network I/O and moves no money.
//
// It exists for two audiences. Operators use it to exercise a Momobase
// deployment end to end — routing, reconciliation, and webhooks — without
// credentials for a real payment provider. Developers use it as the reference
// implementation of [github.com/momobasehq/momobase/providers.PaymentProvider]
// when writing their own adapter.
//
// Every outcome is driven by the provider account's configuration, so a test or
// a demo can force a success, a failure, a settlement delay, or a transport
// error deterministically. See [Config] for the recognized keys.
package dummy
