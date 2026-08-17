import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { AdminPermissions, type AdminUser } from "@momobase/sdk"

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
import { formatDateTime, titleCase } from "@/lib/format"
import { keys } from "@/lib/query-keys"

/** CreateUserDialog collects a new administrator. */
function CreateUserDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { client } = useAuth()
  const queryClient = useQueryClient()
  const [form, setForm] = useState({ name: "", email: "", password: "", role: "" })
  // Roles come from the server, so a role an operator created is assignable here
  // without a client release. The old hardcoded list also offered read_only, which
  // the API rejected.
  const roles = useQuery({ queryKey: keys.authz.roles(), queryFn: () => client.authz.roles() })

  const create = useMutation({
    mutationFn: () => client.users.create(form),
    onSuccess: async (user) => {
      toast.success(`Created ${user.email}`)
      onOpenChange(false)
      setForm({ name: "", email: "", password: "", role: "" })
      await queryClient.invalidateQueries({ queryKey: keys.users.all })
    },
    onError: (error: Error) => toast.error(error.message),
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>New administrator</DialogTitle>
          <DialogDescription>The password is set once here and can be changed later.</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-3">
          <div className="flex flex-col gap-2">
            <Label htmlFor="user-name">Name</Label>
            <Input id="user-name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="user-email">Email</Label>
            <Input id="user-email" type="email" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="user-password">Password</Label>
            <Input id="user-password" type="password" value={form.password} onChange={(e) => setForm({ ...form, password: e.target.value })} />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="user-role">Role</Label>
            <Select value={form.role} onValueChange={(role) => setForm({ ...form, role: role ?? "" })}>
              <SelectTrigger id="user-role">
                <SelectValue placeholder={roles.isPending ? "Loading…" : "Select a role"} />
              </SelectTrigger>
              <SelectContent>
                {(roles.data?.items ?? []).map((role) => (
                  <SelectItem key={role.name} value={role.name}>
                    {titleCase(role.name)}
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
          <Button onClick={() => create.mutate()} disabled={create.isPending || !form.email || !form.password || !form.role}>
            Create
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

/** Users manages administrator accounts and their status. */
export function Users() {
  const { client } = useAuth()
  const queryClient = useQueryClient()
  const paged = usePagedQuery(keys.users.list, (page) => client.users.list(page))
  const [creating, setCreating] = useState(false)
  const [resetting, setResetting] = useState<AdminUser>()
  const [newPassword, setNewPassword] = useState("")

  const changePassword = useMutation({
    mutationFn: () => client.users.changePassword(resetting!.id, newPassword),
    onSuccess: () => {
      toast.success("Password changed — the admin's existing sessions are invalidated")
      setResetting(undefined)
      setNewPassword("")
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const changeStatus = useMutation({
    mutationFn: ({ id, status }: { id: string; status: "active" | "inactive" }) => client.users.changeStatus(id, status),
    onSuccess: async () => {
      toast.success("Status updated")
      await queryClient.invalidateQueries({ queryKey: keys.users.all })
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const columns: Column<AdminUser>[] = [
    { key: "name", header: "Name", cell: (user) => <span className="font-medium">{user.name}</span> },
    { key: "email", header: "Email", cell: (user) => user.email },
    { key: "role", header: "Role", cell: (user) => titleCase(user.role) },
    { key: "status", header: "Status", cell: (user) => <StatusBadge status={user.status} /> },
    { key: "created", header: "Created", cell: (user) => formatDateTime(user.created_at) },
    {
      key: "actions",
      header: "",
      align: "end",
      cell: (user) => (
        <div className="flex justify-end gap-2">
          <GuardedAction permission={AdminPermissions.usersUpdate} variant="outline" size="sm" onClick={() => setResetting(user)}>
            Set password
          </GuardedAction>
          <GuardedAction
            permission={AdminPermissions.usersUpdate}
            variant="outline"
            size="sm"
            disabled={changeStatus.isPending}
            onClick={() => changeStatus.mutate({ id: user.id, status: user.status === "active" ? "inactive" : "active" })}
          >
            {user.status === "active" ? "Deactivate" : "Activate"}
          </GuardedAction>
        </div>
      ),
    },
  ]

  return (
    <Card>
      <CardHeader>
        <CardTitle>Administrators</CardTitle>
        <CardDescription>Console accounts and the roles that gate what they may change.</CardDescription>
        <div className="ms-auto">
          <GuardedAction permission={AdminPermissions.usersCreate} size="sm" onClick={() => setCreating(true)}>
            New administrator
          </GuardedAction>
        </div>
      </CardHeader>
      <CardContent>
        <DataTable columns={columns} rows={paged.items} rowKey={(user) => user.id} loading={paged.loading} />
        <PaginationControls
          page={paged.page}
          perPage={paged.perPage}
          total={paged.total}
          count={paged.count}
          onPageChange={paged.setPage}
          busy={paged.fetching}
        />
      </CardContent>
      <CreateUserDialog open={creating} onOpenChange={setCreating} />

      <Dialog open={Boolean(resetting)} onOpenChange={(open) => !open && setResetting(undefined)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Set password</DialogTitle>
            <DialogDescription>
              Changing {resetting?.email}'s password revokes their active sessions.
            </DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-2">
            <Label htmlFor="new-password">New password</Label>
            <Input
              id="new-password"
              type="password"
              autoComplete="new-password"
              value={newPassword}
              onChange={(event) => setNewPassword(event.target.value)}
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setResetting(undefined)}>
              Cancel
            </Button>
            <Button onClick={() => changePassword.mutate()} disabled={changePassword.isPending || !newPassword}>
              Change password
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  )
}
