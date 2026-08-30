# Administration client

Use `MomobaseAdminClient` for operator workflows such as provisioning applications, configuring providers and routes, and inspecting system health.

## Authenticate and inspect the system

The first request authenticates automatically. You can also authenticate explicitly when a login screen must fail before loading data.

```ts
await admin.authenticate()

const [info, health] = await Promise.all([
  admin.system.info(),
  admin.system.health()
])
```

System methods include `info()`, `health()`, `workers(options?)`, and `runtimeProviders(options?)`.

## Provision an application

```ts
const application = await admin.apps.create({
  name: 'Checkout',
  environment: 'sandbox',
  currency: 'UGX'
})

const credential = await admin.apps.createCredential(application.id, {
  name: 'checkout-server',
  scopes: 'payments:create payments:read'
})

console.log(credential.credential.client_id, credential.client_secret)
```

The secret is returned only when the credential is created or rotated. Store it immediately in a secret manager.

Application methods cover listing, creating, reading, updating, and changing status, plus listing, creating, revoking, and rotating credentials.

## Configure a provider and route

```ts
const account = await admin.providers.createAccount({
  provider_code: 'dummy',
  name: 'Sandbox simulator',
  environment: 'sandbox',
  country: 'UG',
  currency: 'UGX',
  config: { webhook_secret: 'replace-with-a-long-random-secret' }
})

await admin.providers.test(account.id)
await admin.providers.activate(account.id)

await admin.routes.create({
  provider_account_id: account.id,
  service_type: 'collection',
  payment_method: 'momo',
  priority: 100,
  active: true
})
```

Provider methods also expose the adapter registry, settings and configuration updates, balances, and health snapshots. Route methods list routes and update their priority or active state.

## Manage access

Build role and scope interfaces from the server catalogue instead of hard-coding permission strings:

```ts
const permissions = await admin.authz.permissions('admin')
const roles = await admin.authz.roles()
const currentUser = await admin.users.me()
```

`authz` manages roles. `users` lists and creates administrators and changes their password, status, or role.

## Inspect transactions

```ts
const transactions = await admin.transactions.list({ page: 1, perPage: 25 })
const auditLogs = await admin.transactions.auditLogs({ page: 1, perPage: 25 })
const analytics = await admin.analytics.transactions({
  from: '2026-08-01T00:00:00Z',
  to: '2026-09-01T00:00:00Z',
  interval: 'day'
})
```

## Method groups

| Group | Purpose |
| --- | --- |
| `system` | Version, health, workers, and provider runtimes |
| `authz` | Permission catalogue and roles |
| `users` | Administrator accounts |
| `apps` | Applications and credentials |
| `providers` | Provider accounts, configuration, balances, and health |
| `routes` | Routing rules |
| `transactions` | Transactions and audit records |
| `analytics` | Transaction aggregates |

Call `admin.logout()` to invalidate the active administrator session. See [Sessions and tokens](/sdk/sessions) for restoring and clearing sessions.
