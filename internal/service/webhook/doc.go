// Package webhook applies provider callbacks to the transactions they describe.
//
// Handle authenticates the request with a constant-time compare against the
// account's decrypted secret, dedupes on (provider account, payload hash) so a
// redelivery is stored once, and validates the event field-by-field against the
// target transaction before it changes anything.
//
// The event's account is compared exactly against the recorded one, so an adapter
// that normalizes an account in ValidateRequest must report the same form here.
package webhook
