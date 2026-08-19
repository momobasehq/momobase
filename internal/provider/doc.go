// Package provider owns the lifecycle of the payment adapters a deployment runs.
//
// It holds the initialized adapters (RuntimeManager), invokes them behind a timeout
// and a circuit breaker (Executor), administers their accounts and encrypted
// configuration (AdminService), and records their health snapshots (HealthService).
// These live together because the executor and the admin service reach into the
// runtime's unexported state; splitting them would mean exporting a breaker.
//
// It is distinct from the top-level providers package, which is the public contract
// an adapter implements. This package manages instances of that contract.
package provider
