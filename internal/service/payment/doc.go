// Package payment turns a validated request into a recorded transaction.
//
// Orchestrator.Create is the whole path: validate and normalize the payload, look
// up the idempotency key, ask internal/routing for a provider account, let the
// adapter validate the request, persist the transaction and its attempt, execute,
// and apply the result through the transaction state machine.
//
// Accounts are opaque here. The engine checks an account's shape and nothing more —
// no phone, IBAN, or card rules — because what a usable account looks like belongs
// to the selected provider, which decides through providers.RequestValidator.
package payment
