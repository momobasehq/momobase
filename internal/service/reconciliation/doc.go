// Package reconciliation settles transactions the request path left unresolved.
//
// RunOnce re-queries non-terminal transactions against their provider and retries
// webhook events that arrived before their transaction was ready. A transaction
// that cannot be resolved is deferred with an exponential backoff rather than
// retried tightly, and every status change goes through the domain state machine.
//
// It runs as a worker task, so it takes a context and returns on cancellation.
package reconciliation
