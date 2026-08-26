# MTN Mobile Money provider

Register the provider with code `mtn` and configure an account with MTN's API
user/key plus at least one product subscription key:

```json
{
  "api_user": "00000000-0000-4000-8000-000000000000",
  "api_key": "replace-with-api-key",
  "collection_subscription_key": "replace-with-collection-key",
  "disbursement_subscription_key": "replace-with-disbursement-key",
  "target_environment": "sandbox",
  "base_url": "https://sandbox.momodeveloper.mtn.com",
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
