# @momobase/sdk

TypeScript SDK for Momobase.

The public app client uses OAuth `client_credentials` and unwraps standardized API envelopes:

```ts
import { MomobaseClient } from "@momobase/sdk"

const client = new MomobaseClient({ baseUrl, clientId, clientSecret })
const payment = await client.collections.create(payload, { idempotencyKey: "order-1" })
```

The admin client uses OAuth password grant and covers all backend admin endpoints:

```ts
import { MomobaseAdminClient } from "@momobase/sdk"

const admin = new MomobaseAdminClient({ baseUrl, email, password })
await admin.authenticate()
const apps = await admin.apps.list()
```

Successful API responses are expected as:

```json
{ "success": true, "data": {} }
```

List responses unwrap to:

```json
{ "page": 1, "total": 10, "items": [], "count": 10 }
```


## Token handling

The SDK uses the global `fetch()` directly. There is no `fetchImpl` option.

Both clients cache access tokens and call the refresh endpoint when a `refresh_token` is available. They refresh before a known expiry and also refresh and retry once when an API request returns `401`. Concurrent requests share the same refresh operation so rotating refresh tokens are not replayed. If no refresh token exists, the app client requests a new `client_credentials` token and the admin client falls back to password grant when email/password are configured.

## Paying: discover, then charge

Ask what this deployment can serve before collecting any details. The list contains
only methods that would actually route, so a checkout can render it directly:

```ts
const { items } = await app.paymentMethods.list({ serviceType: "collection", country: "UG" })
// [{ service_type: "collection", payment_method: "momo" }]
```

Then post a flat payload, in the order a checkout fills it in:

```ts
await app.collections.create({
  payment_method: "momo",
  scheme: "mtn",
  account: "256770000000",
  amount: 50000,
  currency: "UGX",
  country: "UG",
  reference: "ORDER-1",
  customer: { name: "Ada Lovelace" },
}, { idempotencyKey: "order-1" })
```

`payment_method` and `scheme` come from the chosen method; `account` is what the user entered. `account` may be a mobile number, a bank account, a card token, or a wallet address, and the engine treats it as opaque. What counts as valid is the provider's to decide: an adapter that needs an MSISDN validates and canonicalizes it when the request is routed, and the normalized value is what the transaction records. `scheme` optionally names the network, bank, or card brand, and `metadata` passes provider-specific details through without being persisted.

## Location and fee routing

`country` is a required two-letter ISO code. Each app has one currency, and each provider account has one country and currency. A route is eligible only when all four routing attributes match: service, payment method, country, and currency.

Apps and provider accounts can define separate `collection` and `disbursement` charge rules. A flat rule's value is in minor units; a percentage rule uses basis points (`1000` means 10%). Calculated `platform_fee` and `provider_fee` values are snapshotted when the transaction is created. App APIs expose only `platform_fee`; admin transaction APIs expose both.

## Contract notes

- `payment_method` is `momo`, `card`, `bank_transfer`, or `wallet` and must match an active route.
- `account`, `payment_method`, and `country` are required. `scheme` and `metadata` are optional, as are `customer` and `recipient`, which carry a name and email only.
- Provider country, currency, and charges are changed atomically with `admin.providers.updateSettings(id, settings)`.
- Provider config is one flat provider-owned object and must include `webhook_secret`; location and fee settings live outside it.
- Provider balances use `{ currency, available, ledger }`; active balance queries return one result per active provider account.
- Provider capabilities report `{ service_type, payment_method }`; routes must match both.
