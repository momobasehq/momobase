import { useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"

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
import { AdminPermissions, type ProviderAccount } from "@momobase/sdk"

import { useAuth } from "@/hooks/use-auth"
import { usePagedQuery } from "@/hooks/use-paged-query"
import { formatAmount, formatDateTime } from "@/lib/format"
import { keys } from "@/lib/query-keys"

/** Splits a comma-separated country list into the array the API expects. */
function parseCountries(value: string) {
  return value
    .split(",")
    .map((country) => country.trim().toUpperCase())
    .filter(Boolean)
}

/** CreateAccountDialog registers a provider account for a registered provider code. */
function CreateAccountDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { client } = useAuth()
  const queryClient = useQueryClient()
  const [form, setForm] = useState({ provider_code: "", name: "", environment: "sandbox", countries: "", config: "{\n  \"webhook_secret\": \"\"\n}" })

  // Provider codes come only from the running server's registry. A client-side preset
  // map would silently exclude every out-of-tree adapter, which is the whole point of
  // a pluggable provider interface.
  const registry = useQuery({ queryKey: keys.providers.registry(), queryFn: () => client.providers.registry() })

  const create = useMutation({
    mutationFn: () => {
      let config: Record<string, unknown>
      try {
        config = JSON.parse(form.config) as Record<string, unknown>
      } catch {
        throw new Error("Configuration must be valid JSON")
      }
      return client.providers.createAccount({
        provider_code: form.provider_code,
        name: form.name,
        environment: form.environment as "sandbox" | "production",
        // An account with no countries is unrestricted — the right default for a rail
        // that has no country notion at all.
        countries: parseCountries(form.countries),
        config,
      })
    },
    onSuccess: async (account) => {
      toast.success(`Created ${account.name}`)
      onOpenChange(false)
      await queryClient.invalidateQueries({ queryKey: keys.providers.all })
    },
    onError: (error: Error) => toast.error(error.message),
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>New provider account</DialogTitle>
          <DialogDescription>Accounts are created inactive. Activate one after testing its configuration.</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-3">
          <div className="flex flex-col gap-2">
            <Label htmlFor="provider-code">Provider</Label>
            <Select value={form.provider_code} onValueChange={(code) => setForm({ ...form, provider_code: code ?? "" })}>
              <SelectTrigger id="provider-code">
                <SelectValue placeholder={registry.isPending ? "Loading…" : "Select a registered provider"} />
              </SelectTrigger>
              <SelectContent>
                {(registry.data?.providers ?? []).map((code) => (
                  <SelectItem key={code} value={code}>
                    {code}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="provider-name">Name</Label>
            <Input id="provider-name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="provider-environment">Environment</Label>
            <Select value={form.environment} onValueChange={(env) => setForm({ ...form, environment: env ?? "sandbox" })}>
              <SelectTrigger id="provider-environment">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="sandbox">Sandbox</SelectItem>
                <SelectItem value="production">Production</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="provider-countries">Countries</Label>
            <Input
              id="provider-countries"
              value={form.countries}
              onChange={(e) => setForm({ ...form, countries: e.target.value })}
              placeholder="UG, RW — leave empty for unrestricted"
            />
            <p className="text-muted-foreground">
              An account that declares countries only serves requests naming one of them. Leave it empty for a rail with no country notion.
            </p>
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="provider-config">Configuration (JSON)</Label>
            <Textarea
              id="provider-config"
              rows={6}
              className="font-mono"
              value={form.config}
              onChange={(e) => setForm({ ...form, config: e.target.value })}
            />
            <p className="text-muted-foreground">Encrypted at rest. Must include a long random <code>webhook_secret</code>.</p>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={() => create.mutate()} disabled={create.isPending || !form.provider_code || !form.name}>
            Create
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

/**
 * ConfigureDialog edits an existing account's countries and encrypted configuration.
 *
 * The two are separate API calls because they reload the runtime differently: a config
 * change rebuilds the adapter and rolls the row back if the reload fails, so sending
 * both together would make a rejected config also lose a valid country edit.
 */
function ConfigureDialog({ account, onClose }: { account?: ProviderAccount; onClose: () => void }) {
  const { client } = useAuth()
  const queryClient = useQueryClient()
  const [countries, setCountries] = useState("")
  const [config, setConfig] = useState("")
  const [loaded, setLoaded] = useState<string>()

  // Seed the fields the first time a given account opens the dialog. The stored config
  // is encrypted and never returned, so the box starts empty: submitting replaces it.
  if (account && loaded !== account.id) {
    setLoaded(account.id)
    setCountries(account.countries.join(", "))
    setConfig("")
  }

  async function refresh() {
    await queryClient.invalidateQueries({ queryKey: keys.providers.all })
  }

  const saveCountries = useMutation({
    mutationFn: () => client.providers.updateCountries(account!.id, parseCountries(countries)),
    onSuccess: async () => {
      toast.success("Countries updated")
      await refresh()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const saveConfig = useMutation({
    mutationFn: () => {
      let parsed: Record<string, unknown>
      try {
        parsed = JSON.parse(config) as Record<string, unknown>
      } catch {
        throw new Error("Configuration must be valid JSON")
      }
      return client.providers.updateConfig(account!.id, parsed)
    },
    onSuccess: async () => {
      toast.success("Configuration updated and the runtime reloaded")
      setConfig("")
      await refresh()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const balance = useMutation({
    mutationFn: (country: string) => client.providers.balance(account!.id, country || undefined),
    onSuccess: (result) => toast.success(`Available ${formatAmount(result.available, result.currency)}`),
    onError: (error: Error) => toast.error(error.message),
  })

  return (
    <Dialog open={Boolean(account)} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{account?.name}</DialogTitle>
          <DialogDescription>
            <code>{account?.provider_code}</code> · config v{account?.config_version}
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-col gap-2">
          <Label htmlFor="edit-countries">Countries</Label>
          <div className="flex gap-2">
            <Input
              id="edit-countries"
              value={countries}
              onChange={(event) => setCountries(event.target.value)}
              placeholder="Leave empty for unrestricted"
            />
            <GuardedAction permission={AdminPermissions.providersUpdate} variant="outline" disabled={saveCountries.isPending} onClick={() => saveCountries.mutate()}>
              Save
            </GuardedAction>
          </div>
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor="edit-config">Replacement configuration (JSON)</Label>
          <Textarea
            id="edit-config"
            rows={6}
            className="font-mono"
            value={config}
            onChange={(event) => setConfig(event.target.value)}
            placeholder={'{\n  "webhook_secret": "…"\n}'}
          />
          <p className="text-muted-foreground">
            The stored configuration is encrypted and never returned. Saving replaces it outright and reloads the adapter.
          </p>
          <GuardedAction
            permission={AdminPermissions.providersUpdate}
            className="self-start"
            disabled={saveConfig.isPending || !config.trim()}
            onClick={() => saveConfig.mutate()}
          >
            Replace configuration
          </GuardedAction>
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            disabled={balance.isPending}
            onClick={() => balance.mutate(account?.countries[0] ?? "")}
          >
            Query balance
          </Button>
          <Button onClick={onClose}>Done</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

/** Providers manages provider accounts, their activation, and their balances. */
export function Providers() {
  const { client, can } = useAuth()
  const queryClient = useQueryClient()
  const paged = usePagedQuery(keys.providers.list, (page) => client.providers.list(page))
  // Balances are their own permission, which read_only does not hold, so the card is
  // hidden rather than left to fail — and its query never fires.
  const showBalances = can(AdminPermissions.balancesRead)
  const balances = usePagedQuery(keys.providers.balances, (page) => client.providers.activeBalances(page), 20, showBalances)
  const [creating, setCreating] = useState(false)
  const [configuring, setConfiguring] = useState<ProviderAccount>()

  async function refresh() {
    await queryClient.invalidateQueries({ queryKey: keys.providers.all })
  }

  const toggle = useMutation({
    mutationFn: ({ id, active }: { id: string; active: boolean }) =>
      active ? client.providers.deactivate(id) : client.providers.activate(id),
    onSuccess: async () => {
      toast.success("Provider account updated")
      await refresh()
    },
    onError: (error: Error) => toast.error(error.message),
  })

  const test = useMutation({
    mutationFn: (id: string) => client.providers.test(id),
    onSuccess: () => toast.success("Configuration passed its health check"),
    onError: (error: Error) => toast.error(error.message),
  })

  const columns: Column<ProviderAccount>[] = [
    { key: "name", header: "Name", cell: (account) => <span className="font-medium">{account.name}</span> },
    { key: "code", header: "Provider", cell: (account) => <code>{account.provider_code}</code> },
    { key: "environment", header: "Environment", cell: (account) => <StatusBadge status={account.environment} /> },
    {
      key: "countries",
      header: "Countries",
      cell: (account) =>
        account.countries.length === 0 ? <span className="text-muted-foreground">Unrestricted</span> : account.countries.join(", "),
    },
    { key: "active", header: "Status", cell: (account) => <StatusBadge status={account.active ? "active" : "inactive"} /> },
    { key: "version", header: "Config", cell: (account) => `v${account.config_version}` },
    { key: "updated", header: "Updated", cell: (account) => formatDateTime(account.updated_at) },
    {
      key: "actions",
      header: "",
      align: "end",
      cell: (account) => (
        <div className="flex justify-end gap-2">
          <GuardedAction permission={AdminPermissions.providersTest} variant="outline" size="sm" disabled={test.isPending} onClick={() => test.mutate(account.id)}>
            Test
          </GuardedAction>
          <GuardedAction permission={AdminPermissions.providersUpdate} variant="outline" size="sm" onClick={() => setConfiguring(account)}>
            Configure
          </GuardedAction>
          <GuardedAction
            permission={AdminPermissions.providersUpdate}
            variant={account.active ? "destructive" : "default"}
            size="sm"
            disabled={toggle.isPending}
            onClick={() => toggle.mutate({ id: account.id, active: account.active })}
          >
            {account.active ? "Deactivate" : "Activate"}
          </GuardedAction>
        </div>
      ),
    },
  ]

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>Provider accounts</CardTitle>
          <CardDescription>Configured adapters and the countries each one serves.</CardDescription>
          <div className="ms-auto">
            <GuardedAction permission={AdminPermissions.providersCreate} size="sm" onClick={() => setCreating(true)}>
              New account
            </GuardedAction>
          </div>
        </CardHeader>
        <CardContent>
          <DataTable
            columns={columns}
            rows={paged.items}
            rowKey={(account) => account.id}
            loading={paged.loading}
            empty="No provider accounts yet. Register one to start routing payments."
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

      {showBalances && (
      <Card>
        <CardHeader>
          <CardTitle>Balances</CardTitle>
          <CardDescription>Live balances for every active account, queried per country it serves.</CardDescription>
        </CardHeader>
        <CardContent>
          <DataTable
            columns={[
              { key: "provider", header: "Provider", cell: (row) => <code>{row.provider_code ?? row.provider_account_id}</code> },
              {
                key: "country",
                header: "Country",
                cell: (row) => row.country || <span className="text-muted-foreground">Global</span>,
              },
              { key: "status", header: "Status", cell: (row) => <StatusBadge status={row.status} /> },
              {
                key: "available",
                header: "Available",
                align: "end",
                cell: (row) => (row.balance ? formatAmount(row.balance.available, row.balance.currency) : "—"),
              },
              {
                key: "ledger",
                header: "Ledger",
                align: "end",
                cell: (row) => (row.balance ? formatAmount(row.balance.ledger, row.balance.currency) : "—"),
              },
              { key: "error", header: "Error", cell: (row) => row.error ?? "—" },
            ]}
            rows={balances.items}
            rowKey={(row) => `${row.provider_account_id}-${row.country}`}
            loading={balances.loading}
            empty="No active provider accounts to query."
          />
          <PaginationControls
            page={balances.page}
            perPage={balances.perPage}
            total={balances.total}
            count={balances.count}
            onPageChange={balances.setPage}
            busy={balances.fetching}
          />
        </CardContent>
      </Card>
      )}

      <CreateAccountDialog open={creating} onOpenChange={setCreating} />
      <ConfigureDialog account={configuring} onClose={() => setConfiguring(undefined)} />
    </div>
  )
}
