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

Both clients cache access tokens and call the refresh endpoint when a `refresh_token` is available. If no refresh token exists, the app client requests a new `client_credentials` token and the admin client falls back to password grant when email/password are configured.

## Country-first routing

Collection and disbursement payloads must include `country` as a 2-letter ISO code, for example `UG`. Provider accounts also have a `country` field. Use `GLOBAL` for fallback provider accounts. Momobase routes exact country matches first, then GLOBAL providers, then lowest priority. Phone/MSISDN prefixes are only used for validation.

## Current MVP limits

This SDK now matches the latest backend contract:

- `payment_method` is **only** `momo`.
- Collection requests require `customer` and `momo`.
- Disbursement requests require `recipient` and `momo`.
- Card, bank, and wallet payloads are intentionally not exposed yet.
- Provider account creation requires a `country` value: `GLOBAL` or an ISO-2 country code such as `UG`.
- Provider configs should include `webhook_secret`; `GLOBAL` provider accounts also require `supports_global: true`.
- Provider balances use `{ currency, available, ledger }`; active balance queries return `{ provider_account_id, provider_code, status, balance?, error? }`.
