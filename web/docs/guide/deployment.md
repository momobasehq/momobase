# Deploy Momobase

This guide covers the supplied Docker Compose stack, binary deployment, production configuration, migrations, and operational checks.

Momobase currently targets one serving instance. Review the replica limitation before changing that topology.

## Deploy with Docker Compose

The repository's Compose stack runs Momobase with PostgreSQL.

Create the runtime configuration and generate three independent secrets:

```sh
cp .env.docker.example .env
openssl rand -base64 32
openssl rand -hex 32
openssl rand -hex 32
```

Assign the generated values to:

- `ENCRYPTION_MASTER_KEY_BASE64`;
- `ADMIN_OAUTH_SECRET`; and
- `APP_OAUTH_SECRET`.

Replace the database password, public URL, and CORS origin in `.env`, then apply migrations and start the stack:

```sh
docker compose run --rm momobase migrate
docker compose up -d --build
```

Create the first administrator:

```sh
docker compose exec momobase momobase seed-admin \
  --email admin@example.com \
  --password 'replace-with-a-strong-password' \
  --name 'Super Admin'
```

The service listens on port `9090`, and the Compose configuration enables the dashboard at `/dashboard/`.

## Deploy a binary

Build the binary with `make build` or use a release artifact for your platform. Configure its environment, then run:

```sh
momobase migrate
momobase seed-admin --email admin@example.com --password 'YOUR_STRONG_PASSWORD'
momobase serve
```

The SQLite driver requires cgo. Official release and container builds keep cgo enabled and link the runtime statically.

Run Momobase as an unprivileged user under your platform's service manager. Send `SIGINT` or `SIGTERM` for graceful HTTP and worker shutdown.

## Configure the database

Set `DB_TYPE` to `sqlite`, `postgres`, or `mysql`.

- Use SQLite for local evaluation or a small single-node installation.
- Use PostgreSQL or MySQL when you need their backup, availability, and operational tooling.
- Persist the database outside an ephemeral container filesystem.

## Run migrations safely

Schema setup has two idempotent stages. Ordered migrations handle renames, drops, and backfills; GORM `AutoMigrate` then converges tables with current models.

For local development, `AUTO_MIGRATE=true` applies pending changes during startup. For a controlled deployment, migrate before starting the serving process:

```sh
momobase migrate
AUTO_MIGRATE=false momobase serve
```

If a migration fails, Momobase records it as dirty and stops applying migrations. Inspect and repair the schema before clearing the dirty row in `schema_migrations`.

::: warning Back up before upgrading
Back up the database and encryption key before applying a new release. Schema migrations do not provide an automatic down migration. Restore the backup if an application rollback is incompatible with the migrated schema.
:::

## Configure production settings

Review these settings before exposing Momobase:

| Setting | Purpose |
| --- | --- |
| `APP_ENV` | Set to `staging` or `production` to enable strict validation |
| `APP_ADDR` | Listen address, normally `:9090` |
| `APP_PUBLIC_URL` | Public HTTPS origin |
| `CORS_ALLOWED_ORIGINS` | Explicit browser origins; production rejects `*` |
| `TRUSTED_PROXY_CIDRS` | Proxies whose forwarded client-address chain Momobase may trust |
| `ENCRYPTION_MASTER_KEY_BASE64` | Exactly 32 random bytes, base64 encoded |
| `ADMIN_OAUTH_SECRET` | Administrator-token signing secret |
| `APP_OAUTH_SECRET` | Application-token signing secret |
| `AUTO_MIGRATE` | Whether a serving process applies schema changes |
| `DASHBOARD_ENABLED` | Whether to serve the embedded dashboard |

The full baseline is in [`.env.example`](https://github.com/momobasehq/momobase/blob/main/.env.example). Container-oriented defaults are in [`.env.docker.example`](https://github.com/momobasehq/momobase/blob/main/.env.docker.example).

Keep provider credentials and service secrets outside source control. Back up the encryption key with the database; without it, encrypted provider configuration cannot be recovered.

## Configure a reverse proxy

Terminate HTTPS at a reverse proxy or load balancer. Leave `TRUSTED_PROXY_CIDRS` empty when clients connect directly.

When a proxy is present, list only proxy addresses or CIDRs you control. Momobase reads `X-Forwarded-For` only when the immediate peer is trusted, then walks the chain to the first untrusted address. Trusting arbitrary forwarded headers lets clients evade address-based rate limits.

Add a separately hosted dashboard origin to `CORS_ALLOWED_ORIGINS`. The embedded dashboard needs no separate CORS configuration.

## Verify the deployment

Use the public health endpoints for platform probes:

```text
GET /ping
GET /healthz
```

Use `GET /api/admin/system/health` for authenticated database, provider-runtime, and worker detail.

Monitor failed provider health checks, open circuits, reconciliation errors, unresolved transactions, rejected webhooks, database health, and storage usage.

::: warning Replica limitation
The current runtime is designed for one serving instance. Multiple replicas require deployment-level decisions for worker ownership and provider-runtime updates. Always disable automatic migrations and run one pre-deploy migration job if you evaluate this topology.
:::

## Production checklist

- Use HTTPS and an explicit CORS allowlist.
- Generate unique encryption, OAuth, database, and provider secrets.
- Run migrations as a separate deployment step.
- Persist and test restoration of the database and encryption key.
- Restrict dashboard and Admin API access at the network boundary where possible.
- Configure only trusted reverse-proxy CIDRs.
- Exercise collection, disbursement, webhook, and reconciliation paths with the dummy adapter before enabling a real provider.
