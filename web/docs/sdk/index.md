# TypeScript SDK

`@momobase/sdk` is a typed client for the Momobase application and administration APIs. Use it when a TypeScript service or trusted web application should call Momobase without managing HTTP envelopes and access-token refresh directly.

## Choose a client

| Client | Use it for | Credential |
| --- | --- | --- |
| `MomobaseClient` | Listing payment methods, creating payments, and reading application transactions | Application client ID and secret |
| `MomobaseAdminClient` | Managing operators, applications, providers, routes, and system data | Administrator email and password, or an existing token pair |

Both clients use the global `fetch()` implementation available in modern browsers and Node.js. They return the `data` value from Momobase response envelopes and throw `MomobaseAPIError` for unsuccessful HTTP responses.

::: warning Keep application secrets on a trusted server
`MomobaseClient` holds an application client secret. Do not include that secret in a public browser or mobile bundle. Call it from a backend-for-frontend or another trusted service.
:::

## Continue

- [Install and configure the SDK](/sdk/installation).
- [Create and query payments](/sdk/application-client).
- [Manage Momobase](/sdk/admin-client).
- [Understand sessions and token refresh](/sdk/sessions).
- [Handle errors and cancellation](/sdk/errors).

For endpoints not wrapped by the SDK, use the [API reference](/api-reference).
