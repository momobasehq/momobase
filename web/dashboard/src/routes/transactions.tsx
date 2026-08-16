import { DataTable, type Column } from "@/components/data-table"
import { PaginationControls } from "@/components/pagination-controls"
import { StatusBadge } from "@/components/status-badge"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { useAuth } from "@/hooks/use-auth"
import { usePagedQuery } from "@/hooks/use-paged-query"
import { formatAmount, formatRelative, titleCase } from "@/lib/format"
import { keys } from "@/lib/query-keys"
import type { Transaction } from "@momobase/sdk"

const columns: Column<Transaction>[] = [
  { key: "reference", header: "Reference", cell: (tx) => <span className="font-medium">{tx.reference}</span> },
  { key: "service", header: "Service", cell: (tx) => titleCase(tx.service_type) },
  // Payment methods are free-form strings matched against routes, so this column
  // shows whatever the integrator sent rather than a known set of rails.
  { key: "method", header: "Method", cell: (tx) => <code>{tx.payment_method}</code> },
  { key: "amount", header: "Amount", align: "end", cell: (tx) => formatAmount(tx.amount, tx.currency) },
  { key: "account", header: "Account", cell: (tx) => <code>{tx.customer_account || "—"}</code> },
  { key: "country", header: "Country", cell: (tx) => tx.country || "—" },
  { key: "status", header: "Status", cell: (tx) => <StatusBadge status={tx.status} /> },
  { key: "created", header: "Created", cell: (tx) => formatRelative(tx.created_at) },
]

/** Transactions lists every payment the engine has recorded. */
export function Transactions() {
  const { client } = useAuth()
  const paged = usePagedQuery(keys.transactions.list, (page) => client.transactions.list(page))

  return (
    <Card>
      <CardHeader>
        <CardTitle>Transactions</CardTitle>
        <CardDescription>Collections and disbursements across every app.</CardDescription>
      </CardHeader>
      <CardContent>
        <DataTable
          columns={columns}
          rows={paged.items}
          rowKey={(tx) => tx.id}
          loading={paged.loading}
          empty="No payments have been created yet."
        />
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
