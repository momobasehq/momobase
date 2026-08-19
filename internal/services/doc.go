// Package services implements Momobase's application-level business workflows.
//
// It covers identity and tenancy: administrator authentication and accounts,
// application credentials and their tokens, the authorization catalogue, and
// transaction reporting. The payment path lives in internal/payment, routing in
// internal/routing, adapters in internal/provider, callbacks in internal/webhook,
// and settlement in internal/reconciliation.
package services
