import { useState, type ReactNode } from "react"
import { Link } from "react-router"

import { DataTable, type Column } from "@/components/data-table"
import { PaginationControls } from "@/components/pagination-controls"
import { StatusBadge } from "@/components/status-badge"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { useAuth } from "@/hooks/use-auth"
import { usePagedQuery } from "@/hooks/use-paged-query"
import { formatAmount, formatDateTime, formatRelative, titleCase } from "@/lib/format"
import { keys } from "@/lib/query-keys"
import type { AdminTransaction } from "@momobase/sdk"

const columns: Column<AdminTransaction>[] = [
  { key: "reference", header: "Reference", cell: (tx) => <span className="font-medium">{tx.reference}</span> },
  { key: "service", header: "Service", cell: (tx) => titleCase(tx.service_type) },
  // Payment methods are free-form strings matched against routes, so this column
  // shows whatever the integrator sent rather than a known set of rails.
  { key: "method", header: "Method", cell: (tx) => <code>{tx.payment_method}</code> },
  { key: "amount", header: "Amount", align: "end", cell: (tx) => formatAmount(tx.amount, tx.currency) },
  { key: "provider_fee", header: "Provider fee", align: "end", cell: (tx) => formatAmount(tx.provider_fee, tx.currency) },
  { key: "platform_fee", header: "Platform fee", align: "end", cell: (tx) => formatAmount(tx.platform_fee, tx.currency) },
  { key: "account", header: "Account", cell: (tx) => <code>{tx.customer_account || "—"}</code> },
  { key: "country", header: "Country", cell: (tx) => tx.country || "—" },
  { key: "status", header: "Status", cell: (tx) => <StatusBadge status={tx.status} /> },
  { key: "created", header: "Created", cell: (tx) => formatRelative(tx.created_at) },
]

/** Transactions lists every payment the engine has recorded. */
export function Transactions() {
  const { client } = useAuth()
  const paged = usePagedQuery(keys.transactions.list, (page) => client.transactions.list(page))
  const [selected, setSelected] = useState<AdminTransaction>()

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
          onRowClick={setSelected}
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
      <TransactionDialog transaction={selected} onClose={() => setSelected(undefined)} />
    </Card>
  )
}

function TransactionDialog({ transaction, onClose }: { transaction?: AdminTransaction; onClose: () => void }) {
  return (
    <Dialog open={Boolean(transaction)} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[calc(100svh-2rem)] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{transaction?.reference ?? "Transaction details"}</DialogTitle>
          <DialogDescription>
            Complete values captured on the transaction record. Fees are the immutable amounts computed when it was created.
          </DialogDescription>
        </DialogHeader>

        {transaction && (
          <div className="grid gap-x-8 gap-y-5 sm:grid-cols-2 lg:grid-cols-3">
            <Detail label="Status"><StatusBadge status={transaction.status} /></Detail>
            <Detail label="Service">{titleCase(transaction.service_type)}</Detail>
            <Detail label="Payment method"><code>{transaction.payment_method}</code></Detail>
            <Detail label="Amount">{formatAmount(transaction.amount, transaction.currency)}</Detail>
            <Detail label="Provider fee">{formatAmount(transaction.provider_fee, transaction.currency)}</Detail>
            <Detail label="Platform fee">{formatAmount(transaction.platform_fee, transaction.currency)}</Detail>
            <Detail label="Currency"><code>{transaction.currency}</code></Detail>
            <Detail label="Country"><code>{transaction.country || "—"}</code></Detail>
            <Detail label="Reference"><code className="break-all">{transaction.reference}</code></Detail>
            <Detail label="Transaction ID"><code className="break-all">{transaction.id}</code></Detail>
            <Detail label="App ID"><code className="break-all">{transaction.app_id}</code></Detail>
            <Detail label="Idempotency key"><code className="break-all">{transaction.idempotency_key}</code></Detail>
            <Detail label="Selected route"><code className="break-all">{transaction.selected_route_id || "—"}</code></Detail>
            <Detail label="Provider account">
              {transaction.selected_provider_account_id ? (
                <Link
                  className="break-all font-mono underline-offset-4 hover:underline"
                  to={`/providers/${transaction.selected_provider_account_id}`}
                  onClick={onClose}
                >
                  {transaction.selected_provider_account_id}
                </Link>
              ) : "—"}
            </Detail>
            <Detail label="Provider reference"><code className="break-all">{transaction.provider_reference || "—"}</code></Detail>
            <Detail label="Customer account"><code className="break-all">{transaction.customer_account || "—"}</code></Detail>
            <Detail label="Customer name">{transaction.customer_name || "—"}</Detail>
            <Detail label="Customer email"><span className="break-all">{transaction.customer_email || "—"}</span></Detail>
            <Detail label="Description"><span className="whitespace-pre-wrap">{transaction.description || "—"}</span></Detail>
            <Detail label="Reconciliation attempts">{transaction.reconciliation_attempts}</Detail>
            <Detail label="Last reconciled">{formatDateTime(transaction.last_reconciled_at)}</Detail>
            <Detail label="Next reconciliation">{formatDateTime(transaction.next_reconcile_at)}</Detail>
            <Detail label="Created">{formatDateTime(transaction.created_at)}</Detail>
            <Detail label="Last updated">{formatDateTime(transaction.updated_at)}</Detail>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}

function Detail({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="min-w-0">
      <p className="text-muted-foreground">{label}</p>
      <div className="mt-1">{children}</div>
    </div>
  )
}
