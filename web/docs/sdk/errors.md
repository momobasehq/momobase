# Errors and cancellation

This page covers response errors, local validation, request cancellation, and safe retries in `@momobase/sdk`.

## Handle API errors

Non-successful HTTP responses throw `MomobaseAPIError`:

```ts
import { MomobaseAPIError } from '@momobase/sdk'

try {
  await app.transactions.get('missing-id')
} catch (error) {
  if (error instanceof MomobaseAPIError) {
    console.error(error.status, error.code, error.message, error.body)
  } else {
    throw error
  }
}
```

| Field | Meaning |
| --- | --- |
| `status` | HTTP status code |
| `code` | Stable Momobase error code when the response provides one |
| `message` | Human-readable error message |
| `body` | Parsed response body when one is available |

Network failures and cancellations remain native `fetch()` errors.

## Handle local validation

Before sending a collection or disbursement, the SDK requires:

- a non-empty `payment_method`;
- a non-empty `account`; and
- a two-character `country` code.

These failures throw `Error` without making a network request. Provider-specific account and scheme validation still happens in Momobase after routing.

## Cancel a request

Pass an `AbortSignal` in request options:

```ts
const controller = new AbortController()

const request = app.collections.create(payload, {
  idempotencyKey: 'order-2',
  signal: controller.signal
})

controller.abort()
await request
```

List methods also accept `signal` through their options argument.

## Retry safely

The SDK retries once after `401` because that retry includes a token refresh. It does not retry network failures, timeouts, rate limits, or server errors.

When retrying a collection or disbursement in application code, reuse the same idempotency key and identical payload. Back off before retrying `429` or transient `5xx` responses. Do not automatically retry validation, authorization, or other permanent `4xx` errors.
