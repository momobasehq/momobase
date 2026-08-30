# Install and configure the SDK

This page installs `@momobase/sdk` and creates each available client.

## Requirements

- Node.js 22 or a browser with global `fetch()` and `AbortController` support.
- A reachable Momobase deployment.
- An application credential for `MomobaseClient`, or administrator credentials for `MomobaseAdminClient`.

## Install

```sh
pnpm add @momobase/sdk
```

## Create an application client

```ts
import { MomobaseClient } from '@momobase/sdk'

const app = new MomobaseClient({
  baseUrl: 'https://payments.example.com',
  clientId: process.env.MOMOBASE_CLIENT_ID!,
  clientSecret: process.env.MOMOBASE_CLIENT_SECRET!
})
```

| Option | Required | Description |
| --- | --- | --- |
| `baseUrl` | Yes | Momobase origin, without an API path |
| `clientId` | Yes | Application credential ID |
| `clientSecret` | Yes | Application credential secret |
| `tokenSkewSeconds` | No | Seconds before expiry at which the client refreshes; defaults to `30` |

Keep this client in a trusted runtime because it stores an application secret.

## Create an administration client

```ts
import { MomobaseAdminClient } from '@momobase/sdk'

const admin = new MomobaseAdminClient({
  baseUrl: 'https://payments.example.com',
  email: 'admin@example.com',
  password: process.env.MOMOBASE_ADMIN_PASSWORD!
})
```

| Option | Required | Description |
| --- | --- | --- |
| `baseUrl` | Yes | Momobase origin, without an API path |
| `email`, `password` | Together | Credentials used for password authentication and as a fallback after refresh failure |
| `accessToken` | No | An existing access token |
| `refreshToken` | No | The refresh token paired with `accessToken` |
| `tokenSkewSeconds` | No | Seconds before expiry at which the client refreshes; defaults to `30` |
| `onTokenChange` | No | Callback invoked when the token pair changes or is cleared |

You may start with credentials or restore an existing token pair. See [Sessions and tokens](/sdk/sessions) before persisting administrator tokens in a browser.

## Verify the client

```ts
const health = await admin.system.health()
console.log(health.ok)
```

Continue with the [application client](/sdk/application-client) or [administration client](/sdk/admin-client).
