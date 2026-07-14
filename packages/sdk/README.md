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

## Explicit country routing

Collection and disbursement payloads include `country` as a two-letter ISO code, for example `UG`. Provider accounts expose `countries: string[]`, and Momobase routes only to active accounts whose list contains the transaction country. Route priority decides among eligible providers; no global fallback exists.

Phone/MSISDN values may be local or international. The backend validates them against the transaction country using libphonenumber metadata and normalizes them to E.164 digits.

## Current MVP limits

This SDK now matches the latest backend contract:

- `payment_method` is **only** `momo`.
- Collection requests require `customer` and `momo`.
- Disbursement requests require `recipient` and `momo`.
- Card, bank, and wallet payloads are intentionally not exposed yet.
- Provider account creation requires a non-empty `countries` array such as `["UG", "RW"]`.
- Supported countries can be changed with `admin.providers.updateCountries(id, countries)`.
- `admin.providers.balance(id, country)` requires `country` for a multi-country provider; it may be omitted for a single-country provider.
- Provider configs should include `webhook_secret`; country eligibility is not stored in provider credential config.
- Provider balances use `{ currency, available, ledger }`; active balance queries return one `{ provider_account_id, provider_code, country, status, balance?, error? }` item per supported country.
