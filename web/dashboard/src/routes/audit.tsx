import { DataTable, type Column } from "@/components/data-table"
import { PaginationControls } from "@/components/pagination-controls"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { useAuth } from "@/hooks/use-auth"
import { usePagedQuery } from "@/hooks/use-paged-query"
import { formatDateTime } from "@/lib/format"
import { keys } from "@/lib/query-keys"
import type { AuditLog } from "@momobase/sdk"

const columns: Column<AuditLog>[] = [
  { key: "action", header: "Action", cell: (log) => <code className="font-medium">{log.action}</code> },
  { key: "entity", header: "Entity", cell: (log) => `${log.entity_type || "—"} ${log.entity_id || ""}`.trim() },
  { key: "actor", header: "Actor", cell: (log) => `${log.actor_type || "system"} ${log.actor_id || ""}`.trim() },
  { key: "ip", header: "IP", cell: (log) => <code>{log.ip_address || "—"}</code> },
  { key: "at", header: "When", cell: (log) => formatDateTime(log.created_at) },
]

/** Audit lists administrative and system actions. */
export function Audit() {
  const { client } = useAuth()
  const paged = usePagedQuery(keys.audit.list, (page) => client.transactions.auditLogs(page))

  return (
    <Card>
      <CardHeader>
        <CardTitle>Audit log</CardTitle>
        <CardDescription>Every administrative and system action, most recent first.</CardDescription>
      </CardHeader>
      <CardContent>
        <DataTable columns={columns} rows={paged.items} rowKey={(log) => log.id} loading={paged.loading} empty="No actions recorded yet." />
        <PaginationControls
          page={paged.page}
          perPage={paged.perPage}
          total={paged.total}
          count={paged.count}
          onPageChange={paged.setPage}
          busy={paged.fetching}
        />
      </CardContent>
    </Card>
  )
}
