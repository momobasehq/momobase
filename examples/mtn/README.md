# Run MTN MoMo with the dashboard

You need an MTN MoMo API user, API key, and at least one collection or
disbursement subscription key.

From the repository root, create the local environment and replace the three
security values with newly generated credentials:

```bash
cp .env.example .env
openssl rand -base64 32  # ENCRYPTION_MASTER_KEY_BASE64
openssl rand -hex 32     # ADMIN_OAUTH_SECRET
openssl rand -hex 32     # APP_OAUTH_SECRET
```

Set `DASHBOARD_ENABLED=true` in `.env`. Ensure Redis is available at the
configured `REDIS_ADDR`, then export the environment for the MTN example:

```bash
mkdir -p data
set -a
. ./.env
set +a
```

Build the dashboard and seed its first administrator:

```bash
make dashboard
go run ./cmd/momobase seed-admin \
  --email admin@example.com \
  --password 'replace-with-a-strong-password' \
  --name 'Super Admin'
```

Run Momobase with the MTN provider registered and the dashboard embedded:

```bash
go run -tags dashboard ./examples/mtn
```

Open <http://localhost:9090/dashboard/> and sign in with the administrator
credentials above. Go to **Providers**, select **New account**, choose `mtn`, and
enter the country and currency assigned to the MTN account. Use this configuration
JSON, omitting any product subscription key that is not enabled:

```json
{
  "api_user": "replace-with-mtn-api-user",
  "api_key": "replace-with-mtn-api-key",
  "collection_subscription_key": "replace-with-collection-key",
  "disbursement_subscription_key": "replace-with-disbursement-key",
  "target_environment": "sandbox",
  "base_url": "https://sandbox.momodeveloper.mtn.com",
  "balance_service": "collection",
  "webhook_secret": "replace-with-a-long-random-secret"
}
```

If only disbursement is enabled, change `balance_service` to `disbursement` or
omit it so the provider selects the enabled product. For production, replace
the sandbox URL and target environment with the values supplied during MTN
onboarding.

Generate `webhook_secret` with `openssl rand -hex 32`. After creating the
account, click **Test** to verify the MTN credentials, then **Activate**.
