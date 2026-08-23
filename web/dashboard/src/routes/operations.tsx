import { DataTable, type Column } from "@/components/data-table"
import { PaginationControls } from "@/components/pagination-controls"
import { StatusBadge } from "@/components/status-badge"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { AdminPermissions, type ProviderHealthSnapshot, type RuntimeProvider, type WorkerState } from "@momobase/sdk"

import { useAuth } from "@/hooks/use-auth"
import { usePagedQuery } from "@/hooks/use-paged-query"
import { formatDateTime, titleCase } from "@/lib/format"
import { keys } from "@/lib/query-keys"

const workerColumns: Column<WorkerState>[] = [
  { key: "name", header: "Worker", cell: (worker) => <span className="font-medium">{titleCase(worker.name)}</span> },
  { key: "configured", header: "Configured", cell: (worker) => <StatusBadge status={worker.configured ? "active" : "inactive"} /> },
  { key: "state", header: "State", cell: (worker) => <StatusBadge status={worker.state} /> },
]

const healthColumns: Column<ProviderHealthSnapshot>[] = [
  { key: "account", header: "Provider account", cell: (row) => <code>{row.provider_account_id}</code> },
  { key: "status", header: "Status", cell: (row) => <StatusBadge status={row.status} /> },
  { key: "circuit", header: "Circuit", cell: (row) => <StatusBadge status={row.circuit_state} /> },
  { key: "failures", header: "Failures", align: "end", cell: (row) => row.consecutive_failures },
  { key: "latency", header: "Latency", align: "end", cell: (row) => `${row.latency_ms} ms` },
  { key: "checked", header: "Last checked", cell: (row) => formatDateTime(row.last_checked_at) },
  { key: "error", header: "Last error", cell: (row) => row.last_error_message || "—" },
]

const runtimeColumns: Column<RuntimeProvider>[] = [
  { key: "code", header: "Provider", cell: (row) => <code className="font-medium">{row.provider_code}</code> },
  { key: "account", header: "Account", cell: (row) => <code>{row.provider_account_id}</code> },
  { key: "initialized", header: "Initialized", cell: (row) => <StatusBadge status={row.initialized ? "active" : "inactive"} /> },
  { key: "version", header: "Config", cell: (row) => `v${row.config_version}` },
  { key: "country", header: "Country", cell: (row) => row.country },
  { key: "currency", header: "Currency", cell: (row) => row.currency },
  { key: "capabilities", header: "Capabilities", align: "end", cell: (row) => row.capabilities.length },
]

/** Operations shows worker state, provider health, and the loaded runtimes. */
export function Operations() {
  const { client, can } = useAuth()
  // Reaching this screen needs system:read; the provider sections need their own
  // permission, so a role with one and not the other sees only what it may.
  const showProviders = can(AdminPermissions.providersRead)
  const workers = usePagedQuery(keys.system.workers, (page) => client.system.workers(page))
  const health = usePagedQuery(keys.providers.health, (page) => client.providers.health(page), 20, showProviders)
  const runtime = usePagedQuery(keys.system.runtime, (page) => client.system.runtimeProviders(page), 20, showProviders)

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>Workers</CardTitle>
          <CardDescription>Background loops owned by the worker manager.</CardDescription>
        </CardHeader>
        <CardContent>
          <DataTable columns={workerColumns} rows={workers.items} rowKey={(worker) => worker.name} loading={workers.loading} />
          <PaginationControls
            page={workers.page}
            perPage={workers.perPage}
            total={workers.total}
            count={workers.count}
            onPageChange={workers.setPage}
            busy={workers.fetching}
          />
        </CardContent>
      </Card>

      {showProviders && (
      <Card>
        <CardHeader>
          <CardTitle>Provider health</CardTitle>
          <CardDescription>The health probe's latest verdict, and each account's circuit breaker.</CardDescription>
        </CardHeader>
        <CardContent>
          <DataTable
            columns={healthColumns}
            rows={health.items}
            rowKey={(row) => row.provider_account_id}
            loading={health.loading}
            empty="No health snapshots recorded yet."
          />
          <PaginationControls
            page={health.page}
            perPage={health.perPage}
            total={health.total}
            count={health.count}
            onPageChange={health.setPage}
            busy={health.fetching}
          />
        </CardContent>
      </Card>
      )}

      {showProviders && (
      <Card>
        <CardHeader>
          <CardTitle>Runtime providers</CardTitle>
          <CardDescription>Adapters currently held in memory.</CardDescription>
        </CardHeader>
        <CardContent>
          <DataTable
            columns={runtimeColumns}
            rows={runtime.items}
            rowKey={(row) => row.provider_account_id}
            loading={runtime.loading}
            empty="No adapters are loaded."
          />
          <PaginationControls
            page={runtime.page}
            perPage={runtime.perPage}
            total={runtime.total}
            count={runtime.count}
            onPageChange={runtime.setPage}
            busy={runtime.fetching}
          />
        </CardContent>
      </Card>
      )}
    </div>
  )
}
