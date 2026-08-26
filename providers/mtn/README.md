# MTN Mobile Money provider

Register the provider with code `mtn`. The provider account's environment controls
how MTN credentials are loaded.

## Sandbox

Select `sandbox` as the provider account environment and supply at least one MTN
MoMo sandbox product subscription key:

```json
{
  "collection_subscription_key": "replace-with-collection-key",
  "disbursement_subscription_key": "replace-with-disbursement-key",
  "balance_service": "collection",
  "webhook_secret": "replace-with-long-random-secret"
}
```

During initialization, the provider creates an MTN sandbox API user and API key.
It defaults to `https://sandbox.momodeveloper.mtn.com` with the target environment
`sandbox`. Set `provider_callback_host` when MTN should register a specific callback
domain; otherwise the provider uses the host from `callback_url`, then `localhost`.

Supplying both `api_user` and `api_key` in sandbox mode reuses those credentials
instead of provisioning another user. Supplying only one is rejected.

## Production

Select `production` as the provider account environment. Production does not
provision credentials: provide the API user, API key, base URL, target environment,
and at least one product subscription key issued during MTN onboarding:

```json
{
  "api_user": "replace-with-production-api-user",
  "api_key": "replace-with-production-api-key",
  "collection_subscription_key": "replace-with-collection-key",
  "disbursement_subscription_key": "replace-with-disbursement-key",
  "target_environment": "replace-with-production-target",
  "base_url": "https://replace-with-production-api-host",
  "balance_service": "collection",
  "webhook_secret": "replace-with-long-random-secret"
}
```

`collection_subscription_key` enables collections and
`disbursement_subscription_key` enables disbursements. When both are enabled,
`balance_service` selects the product account returned by balance queries. The
default is `collection` when available.

Customer accounts must be international MSISDNs: 8 to 15 digits including the
country calling code. Formatting such as `+256 770 000 000` is accepted and
stored as `256770000000`.

Set `callback_url` only when callbacks should be requested. MTN requires its
host to match the callback host registered for the API user. Momobase also
requires `X-Webhook-Secret` on inbound callbacks, so a direct MTN callback must
pass through a trusted gateway that adds the configured `webhook_secret` header.
Without a callback URL, Momobase's reconciliation worker polls MTN's transaction
status endpoint.

MTN aggregator accounts may set `transfer_type`; ordinary accounts should leave
it unset.
