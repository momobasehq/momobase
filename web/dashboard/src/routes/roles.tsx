import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Lock } from "lucide-react"
import { toast } from "sonner"
import { AdminPermissions, PermissionWildcard, type Permission, type Role } from "@momobase/sdk"

import { DataTable, type Column } from "@/components/data-table"
import { GuardedAction } from "@/components/guarded-action"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import { useAuth } from "@/hooks/use-auth"
import { titleCase } from "@/lib/format"
import { keys } from "@/lib/query-keys"

/** Groups permissions by the resource before the colon, which is how operators read them. */
function byResource(permissions: Permission[]) {
  const groups = new Map<string, Permission[]>()
  for (const permission of permissions) {
    const resource = permission.code.split(":")[0] ?? permission.code
    groups.set(resource, [...(groups.get(resource) ?? []), permission])
  }
  return [...groups.entries()].sort(([a], [b]) => a.localeCompare(b))
}

interface EditorProps {
  /** The role being edited, or undefined when creating one. */
  role?: Role
  open: boolean
  onOpenChange: (open: boolean) => void
}

/** RoleEditor creates a role or replaces an existing one's permission set. */
function RoleEditor({ role, open, onOpenChange }: EditorProps) {
  const { client } = useAuth()
  const queryClient = useQueryClient()
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [selected, setSelected] = useState<string[]>([])
  const [seeded, setSeeded] = useState<string>()

  // Only administrative permissions can be held by a role; app-audience codes belong
  // to credentials, and the server refuses them here.
  const catalogue = useQuery({
    queryKey: keys.authz.permissions("admin"),
    queryFn: () => client.authz.permissions("admin"),
    enabled: open,
  })

  // Seed the form once per role the dialog opens for, so typing is not overwritten by
  // a refetch.
  const identity = role?.name ?? "__new__"
  if (open && seeded !== identity) {
    setSeeded(identity)
    setName(role?.name ?? "")
    setDescription(role?.description ?? "")
    setSelected(role?.permissions.map((permission) => permission.code) ?? [])
  }
  if (!open && seeded !== undefined) setSeeded(undefined)

  const save = useMutation({
    mutationFn: () =>
      role
        ? client.authz.updateRole(role.name, { description, permissions: selected })
        : client.authz.createRole({ name, description, permissions: selected }),
    onSuccess: async () => {
      toast.success(role ? `Updated ${role.name}` : `Created ${name}`)
      onOpenChange(false)
      await queryClient.invalidateQueries({ queryKey: keys.authz.all })
    },
    onError: (error: Error) => toast.error(error.message),
  })

  function toggle(code: string, checked: boolean) {
    setSelected((current) => (checked ? [...current, code] : current.filter((held) => held !== code)))
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{role ? `Edit ${role.name}` : "New role"}</DialogTitle>
          <DialogDescription>
            A role grants exactly the permissions selected here. Administrators holding it gain them on their next
            request, without signing in again.
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-3">
          {!role && (
            <div className="flex flex-col gap-2">
              <Label htmlFor="role-name">Name</Label>
              <Input id="role-name" value={name} onChange={(event) => setName(event.target.value)} placeholder="support" />
              <p className="text-muted-foreground">
                A role's name is its identity, because administrators refer to it by name. It cannot be changed later.
              </p>
            </div>
          )}
          <div className="flex flex-col gap-2">
            <Label htmlFor="role-description">Description</Label>
            <Input
              id="role-description"
              value={description}
              onChange={(event) => setDescription(event.target.value)}
              placeholder="Read transactions and reissue credentials"
            />
          </div>
        </div>

        <div className="max-h-80 overflow-y-auto">
          {catalogue.isPending ? (
            <Skeleton className="h-40 w-full" />
          ) : (
            <div className="flex flex-col gap-4">
              {byResource(catalogue.data?.items ?? []).map(([resource, permissions]) => (
                <div key={resource} className="flex flex-col gap-2">
                  <p className="font-medium">{titleCase(resource)}</p>
                  {permissions.map((permission) => (
                    <label key={permission.id} className="flex items-start gap-2">
                      <Checkbox
                        checked={selected.includes(permission.code)}
                        onCheckedChange={(checked) => toggle(permission.code, checked === true)}
                      />
                      <span>
                        <code>{permission.code}</code>
                        <span className="text-muted-foreground"> — {permission.description}</span>
                      </span>
                    </label>
                  ))}
                </div>
              ))}
            </div>
          )}
        </div>

        <DialogFooter>
          <p className="text-muted-foreground me-auto">{selected.length} selected</p>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={() => save.mutate()} disabled={save.isPending || (!role && !name)}>
            {role ? "Save" : "Create"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

/** Roles lists roles and edits the permissions each one grants. */
export function Roles() {
  const { client } = useAuth()
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState<Role>()
  const [creating, setCreating] = useState(false)

  const roles = useQuery({ queryKey: keys.authz.roles(), queryFn: () => client.authz.roles() })

  const remove = useMutation({
    mutationFn: (name: string) => client.authz.deleteRole(name),
    onSuccess: async () => {
      toast.success("Role deleted")
      await queryClient.invalidateQueries({ queryKey: keys.authz.all })
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const columns: Column<Role>[] = [
    {
      key: "name",
      header: "Role",
      cell: (role) => (
        <span className="flex items-center gap-2 font-medium">
          {titleCase(role.name)}
          {/* System roles are seeded and re-synchronised on every start, which is how a
              permission added by a new release reaches super_admin. Editing one would
              be reverted, so it is refused rather than silently undone. */}
          {role.system && (
            <Badge variant="outline">
              <Lock />
              System
            </Badge>
          )}
        </span>
      ),
    },
    { key: "description", header: "Description", cell: (role) => role.description || "—" },
    {
      key: "permissions",
      header: "Permissions",
      cell: (role) =>
        role.permissions.some((permission) => permission.code === PermissionWildcard) ? (
          <span>Everything, including future permissions</span>
        ) : (
          <span>{role.permissions.length}</span>
        ),
    },
    {
      key: "actions",
      header: "",
      align: "end",
      cell: (role) => (
        <div className="flex justify-end gap-2">
          <GuardedAction
            permission={AdminPermissions.rolesUpdate}
            variant="outline"
            size="sm"
            disabled={role.system}
            onClick={() => setEditing(role)}
          >
            Edit
          </GuardedAction>
          <GuardedAction
            permission={AdminPermissions.rolesDelete}
            variant="destructive"
            size="sm"
            disabled={role.system || remove.isPending}
            onClick={() => remove.mutate(role.name)}
          >
            Delete
          </GuardedAction>
        </div>
      ),
    },
  ]

  return (
    <Card>
      <CardHeader>
        <CardTitle>Roles</CardTitle>
        <CardDescription>
          What each role of administrator may do. Seeded roles are read-only; create one to grant a different set.
        </CardDescription>
        <div className="ms-auto">
          <GuardedAction permission={AdminPermissions.rolesCreate} size="sm" onClick={() => setCreating(true)}>
            New role
          </GuardedAction>
        </div>
      </CardHeader>
      <CardContent>
        <DataTable columns={columns} rows={roles.data?.items ?? []} rowKey={(role) => role.id} loading={roles.isPending} />
      </CardContent>

      <RoleEditor open={creating} onOpenChange={setCreating} />
      <RoleEditor role={editing} open={Boolean(editing)} onOpenChange={(open) => !open && setEditing(undefined)} />
    </Card>
  )
}
