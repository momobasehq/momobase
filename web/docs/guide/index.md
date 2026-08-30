# Understand Momobase

Momobase is a payment orchestration service. Your applications send collections and disbursements to one API, and Momobase routes each request through an eligible provider account.

This page explains the runtime model. Use [Run Momobase locally](/guide/getting-started) when you want a working development instance.

## Runtime model

A Momobase deployment has four main parts:

| Part | Responsibility |
| --- | --- |
| HTTP API | Authenticates applications and administrators, validates requests, and exposes payment and operational endpoints |
| Payment services | Apply idempotency, select routes, call providers, and persist legal transaction transitions |
| Provider runtimes | Hold initialized provider adapters, health state, and circuit-breaker state for active accounts |
| Workers | Check provider health, reconcile unresolved transactions, and remove expired sessions |

The supplied binary also embeds the administration dashboard. You can run that binary as a service or [embed Momobase in a Go application](/guide/embedding).

## Payment flow

1. An application exchanges a client ID and secret for an access token.
2. The application lists the payment methods that can currently route.
3. It creates a collection or disbursement with an `Idempotency-Key`.
4. Momobase normalizes the request and checks for an existing idempotent result.
5. Payment-request hooks run before routing or persistence.
6. Routing selects the highest-ranked active route whose provider account matches the service, payment method, country, and currency.
7. The provider validates provider-specific account data before Momobase writes the transaction and attempt.
8. Momobase calls the provider with a bounded context and persists the normalized result.
9. A webhook or reconciliation check may advance an unresolved transaction later.

Provider network calls never run inside database transactions. Transaction status changes pass through one state machine, regardless of whether the change comes from the request, a webhook, or reconciliation.

## Provider model

A provider account combines an adapter code with encrypted credentials, one country, one currency, fee rules, and an environment. A route connects that account to a service and payment method.

A route is usable only when:

- the route and provider account are active;
- the adapter declares the matching service and payment method;
- the account country and currency match the payment;
- an initialized provider runtime is available; and
- health and circuit-breaker state allow the request.

Provider-specific account formats stay in adapters. Momobase treats `account` as an opaque value until the selected provider validates and optionally normalizes it.

## Persistence and recovery

Momobase stores applications, credentials, routing configuration, transactions, provider attempts, webhook deliveries, and audit records in SQLite, PostgreSQL, or MySQL.

Versioned migrations handle schema changes that require renames, drops, or backfills. GORM `AutoMigrate` then converges the schema with current models. See [Deploy Momobase](/guide/deployment) for migration and backup guidance.

## Choose your next task

- [Run Momobase locally](/guide/getting-started).
- [Deploy Momobase](/guide/deployment).
- [Add payment hooks](/guide/extensions).
- [Build a provider adapter](/guide/providers).
- [Embed Momobase as a Go package](/guide/embedding).
- [Develop and test the repository](/guide/development).
