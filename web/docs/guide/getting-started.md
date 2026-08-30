# Run Momobase locally

This quickstart builds Momobase with SQLite, creates the first administrator, enables the dashboard, and submits a simulated collection.

## Prerequisites

- The Go toolchain declared in `go.mod`.
- Node.js 22 or later.
- pnpm 11.
- A C compiler for the SQLite driver.
- Git and Make.

Use the [deployment guide](/guide/deployment) instead when you want a container or production-oriented setup.

## Build Momobase

```sh
git clone https://github.com/momobasehq/momobase.git
cd momobase
cp .env.example .env
make build
```

`make build` installs the web workspace, builds the embedded dashboard, and writes `bin/momobase`.

## Initialize the database

Apply the schema and create a super administrator:

```sh
./bin/momobase migrate
./bin/momobase seed-admin \
  --email admin@example.com \
  --password 'replace-with-a-strong-password' \
  --name 'Super Admin'
```

The development configuration uses SQLite at `./data/momobase.db`.

## Enable the dashboard

Set these values in `.env`:

```dotenv
DASHBOARD_ENABLED=true
DASHBOARD_PATH=/dashboard
```

Start Momobase:

```sh
./bin/momobase serve
```

Open `http://localhost:9090/dashboard/` and sign in with the administrator you created.

## Verify the service

```sh
curl http://localhost:9090/healthz
```

A healthy service returns an HTTP `200` response.

## Configure a simulated route

In the dashboard, create these resources in order:

1. Create an application with currency `UGX`.
2. Create an application credential with the scopes your client needs. Save the secret when it appears; Momobase stores only its hash.
3. Create a `dummy` provider account for country `UG` and currency `UGX` with this configuration:

   ```json
   {
     "webhook_secret": "replace-with-a-long-random-secret"
   }
   ```

4. Activate the provider account.
5. Create an active collection route for the `momo` payment method.

The dummy adapter performs no network I/O and moves no money.

## Submit a collection

Request an application token:

```sh
curl -X POST http://localhost:9090/api/v1/token \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  --data-urlencode 'grant_type=client_credentials' \
  --data-urlencode 'client_id=YOUR_CLIENT_ID' \
  --data-urlencode 'client_secret=YOUR_CLIENT_SECRET'
```

Copy the returned `access_token`, then create a collection:

```sh
curl -X POST http://localhost:9090/api/v1/collections \
  -H 'Authorization: Bearer YOUR_ACCESS_TOKEN' \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: order-1' \
  -d '{
    "payment_method": "momo",
    "scheme": "mtn",
    "account": "256770000000",
    "amount": 50000,
    "currency": "UGX",
    "country": "UG",
    "reference": "ORDER-1",
    "customer": { "name": "Ada Lovelace", "email": "ada@example.com" }
  }'
```

The response contains a Momobase transaction ID and the status returned by the dummy provider. Reuse the same idempotency key only for retries of this exact request.

## Next steps

- Use the [TypeScript application client](/sdk/application-client).
- Explore every endpoint in the [API reference](/api-reference).
- Review the [deployment checklist](/guide/deployment#production-checklist).
