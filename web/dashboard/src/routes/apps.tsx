import { useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { ChevronRight } from "lucide-react"
import { Link } from "react-router"
import { toast } from "sonner"
import { AdminPermissions, type App } from "@momobase/sdk"

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
import { Textarea } from "@/components/ui/textarea"
import { useAuth } from "@/hooks/use-auth"
import { usePagedQuery } from "@/hooks/use-paged-query"
import { formatDateTime, titleCase } from "@/lib/format"
import { keys } from "@/lib/query-keys"

const environments = ["sandbox", "production"] as const

/** CreateAppDialog registers a new integrating application. */
function CreateAppDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { client } = useAuth()
  const queryClient = useQueryClient()
  const [form, setForm] = useState<{ name: string; description: string; environment: (typeof environments)[number] }>({
    name: "",
    description: "",
    environment: "sandbox",
  })

  const create = useMutation({
    mutationFn: () => client.apps.create(form),
    onSuccess: async (app) => {
      toast.success(`Created ${app.name}`)
      onOpenChange(false)
      setForm({ name: "", description: "", environment: "sandbox" })
      await queryClient.invalidateQueries({ queryKey: keys.apps.all })
    },
    onError: (error: Error) => toast.error(error.message),
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>New app</DialogTitle>
          <DialogDescription>Apps create and query payments through the public API.</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-3">
          <div className="flex flex-col gap-2">
            <Label htmlFor="app-name">Name</Label>
            <Input id="app-name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="app-description">Description</Label>
            <Textarea id="app-description" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="app-environment">Environment</Label>
            <Select
              value={form.environment}
              onValueChange={(value) => setForm({ ...form, environment: (value as (typeof environments)[number]) ?? "sandbox" })}
            >
              <SelectTrigger id="app-environment">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {environments.map((environment) => (
                  <SelectItem key={environment} value={environment}>
                    {titleCase(environment)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={() => create.mutate()} disabled={create.isPending || !form.name}>
            Create
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

/** Apps lists integrating applications and links to their credentials. */
export function Apps() {
  const { client } = useAuth()
  const paged = usePagedQuery(keys.apps.list, (page) => client.apps.list(page))
  const [creating, setCreating] = useState(false)


  const columns: Column<App>[] = [
    {
      key: "name",
      header: "Name",
      // Underlined unconditionally: with hover-only underline this read as plain text,
      // so the detail page — and every credential on it — looked like it did not exist.
      cell: (app) => (
        <Link to={`/apps/${app.id}`} className="font-medium underline underline-offset-4">
          {app.name}
        </Link>
      ),
    },
    { key: "environment", header: "Environment", cell: (app) => <StatusBadge status={app.environment} /> },
    { key: "status", header: "Status", cell: (app) => <StatusBadge status={app.status} /> },
    { key: "description", header: "Description", cell: (app) => app.description || "—" },
    { key: "created", header: "Created", cell: (app) => formatDateTime(app.created_at) },
    {
      key: "actions",
      header: "",
      align: "end",
      // Status, credentials, and the tester all live on the detail page, so the list
      // offers one unambiguous way in rather than a partial set of actions.
      cell: (app) => (
        <Button variant="outline" size="sm" render={<Link to={`/apps/${app.id}`} />}>
          Manage
          <ChevronRight />
        </Button>
      ),
    },
  ]

  return (
    <Card>
      <CardHeader>
        <CardTitle>Apps</CardTitle>
        <CardDescription>Systems that create payments through the public API.</CardDescription>
        <div className="ms-auto">
          <GuardedAction permission={AdminPermissions.appsCreate} size="sm" onClick={() => setCreating(true)}>
            New app
          </GuardedAction>
        </div>
      </CardHeader>
      <CardContent>
        <DataTable columns={columns} rows={paged.items} rowKey={(app) => app.id} loading={paged.loading} />
        <PaginationControls
          page={paged.page}
          perPage={paged.perPage}
          total={paged.total}
          count={paged.count}
          onPageChange={paged.setPage}
          busy={paged.fetching}
        />
      </CardContent>
      <CreateAppDialog open={creating} onOpenChange={setCreating} />
    </Card>
  )
}
