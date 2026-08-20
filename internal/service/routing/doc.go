// Package routing decides which provider account executes a payment.
//
// Engine performs the selection on the request path: it picks the lowest-priority
// active route whose account is active, declares the service, is eligible for the
// request country, and whose circuit and health snapshot are not open or down.
// AdminService is the operator-facing side, creating and reprioritizing routes.
//
// AvailablePaymentMethods reuses the engine's own candidate check, so the methods a
// client is offered cannot drift from what would actually route.
package routing
