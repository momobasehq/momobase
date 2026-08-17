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

## Country routing

`country` is an optional two-letter ISO code. Provider accounts expose `countries: string[]`: an account that lists countries serves only requests naming one of them, and an account with an empty list is unrestricted, which is how a rail with no country notion is modelled. Route priority decides among eligible providers; there is no global fallback country.

## Contract notes

- `payment_method` is free-form and must match an active payment route. `momo` is a convention, not an enum.
- `account` and `payment_method` are required. `scheme`, `metadata`, and `country` are optional, as are `customer` and `recipient`, which carry a name and email only.
- Supported countries can be changed with `admin.providers.updateCountries(id, countries)`; an empty array leaves the account unrestricted.
- `admin.providers.balance(id, country)` requires `country` only for a provider that declares more than one.
- Provider configs should include `webhook_secret`; country eligibility is not stored in provider credential config.
- Provider balances use `{ currency, available, ledger }`; active balance queries return one `{ provider_account_id, provider_code, country, status, balance?, error? }` item per supported country, or one item with an empty `country` for an unrestricted provider.
- Provider capabilities report the service only — `{ service_type }`. Which rails reach an account is decided by its routes.
