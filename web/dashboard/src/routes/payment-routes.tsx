import { useMemo, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { AdminPermissions, type PaymentRoute, type ServiceType } from "@momobase/sdk"

import { DataTable, type Column } from "@/components/data-table"
import { GuardedAction } from "@/components/guarded-action"
import { PaginationControls } from "@/components/pagination-controls"
import { StatusBadge } from "@/components/status-badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { useAuth } from "@/hooks/use-auth"
import { usePagedQuery } from "@/hooks/use-paged-query"
import { keys } from "@/lib/query-keys"
import { titleCase } from "@/lib/format"

/** CreateRouteDialog connects a provider account to a service and payment method. */
function CreateRouteDialog({ open, onOpenChange, methods }: { open: boolean; onOpenChange: (open: boolean) => void; methods: string[] }) {
  const { client } = useAuth()
  const queryClient = useQueryClient()
  const [form, setForm] = useState({ service_type: "collection" as ServiceType, payment_method: "", provider_account_id: "", priority: 1, active: true })

  const accounts = useQuery({
    queryKey: keys.providers.list({ page: 1, perPage: 100 }),
    queryFn: () => client.providers.list({ page: 1, perPage: 100 }),
  })

  const create = useMutation({
    mutationFn: () => client.routes.create(form),
    onSuccess: async () => {
      toast.success("Route created")
      onOpenChange(false)
      await queryClient.invalidateQueries({ queryKey: keys.routes.all })
    },
    onError: (error: Error) => toast.error(error.message),
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>New route</DialogTitle>
          <DialogDescription>The lowest-priority active route whose account is eligible wins.</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-3">
          <div className="flex flex-col gap-2">
            <Label htmlFor="route-service">Service</Label>
            <Select
              value={form.service_type}
              onValueChange={(value) => setForm({ ...form, service_type: (value as ServiceType) ?? "collection" })}
            >
              <SelectTrigger id="route-service">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="collection">Collection</SelectItem>
                <SelectItem value="disbursement">Disbursement</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="route-method">Payment method</Label>
            {/* Free text, not a dropdown: payment methods are arbitrary strings that
                only ever get compared against a route. The datalist suggests the ones
                already in use without preventing a new rail from being named. */}
            <Input
              id="route-method"
              list="known-payment-methods"
              value={form.payment_method}
              onChange={(event) => setForm({ ...form, payment_method: event.target.value })}
              placeholder="momo, bank_transfer, card…"
            />
            <datalist id="known-payment-methods">
              {methods.map((method) => (
                <option key={method} value={method} />
              ))}
            </datalist>
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="route-account">Provider account</Label>
            <Select value={form.provider_account_id} onValueChange={(id) => setForm({ ...form, provider_account_id: id ?? "" })}>
              <SelectTrigger id="route-account">
                <SelectValue placeholder={accounts.isPending ? "Loading…" : "Select an account"} />
              </SelectTrigger>
              <SelectContent>
                {(accounts.data?.items ?? []).map((account) => (
                  <SelectItem key={account.id} value={account.id}>
                    {account.name} ({account.provider_code})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="route-priority">Priority</Label>
            <Input
              id="route-priority"
              type="number"
              min={1}
              value={form.priority}
              onChange={(event) => setForm({ ...form, priority: Number(event.target.value) || 1 })}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            onClick={() => create.mutate()}
            disabled={create.isPending || !form.payment_method || !form.provider_account_id}
          >
            Create
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

/** PaymentRoutes manages the routing table. */
export function PaymentRoutes() {
  const { client } = useAuth()
  const queryClient = useQueryClient()
  const paged = usePagedQuery(keys.routes.list, (page) => client.routes.list(page))
  const [creating, setCreating] = useState(false)

  const methods = useMemo(
    () => [...new Set(paged.items.map((route) => route.payment_method))].sort(),
    [paged.items],
  )

  const update = useMutation({
    mutationFn: ({ id, priority, active }: { id: string; priority: number; active: boolean }) =>
      client.routes.update(id, { priority, active }),
    onSuccess: async () => {
      toast.success("Route updated")
      await queryClient.invalidateQueries({ queryKey: keys.routes.all })
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const columns: Column<PaymentRoute>[] = [
    { key: "service", header: "Service", cell: (route) => titleCase(route.service_type) },
    { key: "method", header: "Method", cell: (route) => <code className="font-medium">{route.payment_method}</code> },
    { key: "account", header: "Provider account", cell: (route) => <code>{route.provider_account_id}</code> },
    { key: "priority", header: "Priority", align: "end", cell: (route) => route.priority },
    { key: "active", header: "Status", cell: (route) => <StatusBadge status={route.active ? "active" : "inactive"} /> },
    {
      key: "actions",
      header: "",
      align: "end",
      cell: (route) => (
        <GuardedAction
          permission={AdminPermissions.routesUpdate}
          variant="outline"
          size="sm"
          disabled={update.isPending}
          onClick={() => update.mutate({ id: route.id, priority: route.priority, active: !route.active })}
        >
          {route.active ? "Disable" : "Enable"}
        </GuardedAction>
      ),
    },
  ]

  return (
    <Card>
      <CardHeader>
        <CardTitle>Routes</CardTitle>
        <CardDescription>Which provider account serves each service and payment method.</CardDescription>
        <div className="ms-auto">
          <GuardedAction permission={AdminPermissions.routesCreate} size="sm" onClick={() => setCreating(true)}>
            New route
          </GuardedAction>
        </div>
      </CardHeader>
      <CardContent>
        <DataTable
          columns={columns}
          rows={paged.items}
          rowKey={(route) => route.id}
          loading={paged.loading}
          empty="No routes yet. A payment cannot be executed until one exists."
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
      <CreateRouteDialog open={creating} onOpenChange={setCreating} methods={methods} />
    </Card>
  )
}
