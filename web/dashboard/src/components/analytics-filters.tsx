import { useQuery } from "@tanstack/react-query"
import { AdminPermissions, type AnalyticsQuery } from "@momobase/sdk"

import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { useAuth } from "@/hooks/use-auth"
import { keys } from "@/lib/query-keys"

/** The sentinel a Select uses for "no filter", since an empty value clears a Select. */
const anyValue = "__any__"

/** Ranges offered as presets. Custom bounds are typed into the date inputs. */
const ranges = [
  { value: "7", label: "Last 7 days" },
  { value: "30", label: "Last 30 days" },
  { value: "90", label: "Last 90 days" },
] as const

export interface AnalyticsRange {
  /** Inclusive RFC3339 start. */
  from: string
  /** Exclusive RFC3339 end. */
  to: string
  interval: "day" | "hour"
  appId?: string
  providerAccountId?: string
}

/** defaultRange is the last 30 days by day, matching the server's own default. */
export function defaultRange(): AnalyticsRange {
  const to = new Date()
  const from = new Date(to)
  from.setDate(from.getDate() - 30)
  return { from: from.toISOString(), to: to.toISOString(), interval: "day" }
}

/** Converts the range into the SDK's query shape. */
export function toQuery(range: AnalyticsRange): AnalyticsQuery {
  return {
    from: range.from,
    to: range.to,
    interval: range.interval,
    appId: range.appId,
    providerAccountId: range.providerAccountId,
  }
}

/** Renders an RFC3339 instant as the yyyy-MM-dd a date input expects. */
function asDateInput(iso: string) {
  return iso.slice(0, 10)
}

interface AnalyticsFiltersProps {
  range: AnalyticsRange
  onChange: (range: AnalyticsRange) => void
  /** Hides the app selector where the app is already fixed by the page. */
  showApp?: boolean
  /** Hides the provider selector where the provider is already fixed by the page. */
  showProvider?: boolean
}

/**
 * AnalyticsFilters is the one row of controls above the charts.
 *
 * Every chart on a screen reads the same range, so a comparison between them is always
 * of the same window — per-chart filters are how dashboards end up showing two periods
 * side by side and inviting a false conclusion.
 */
export function AnalyticsFilters({ range, onChange, showApp = true, showProvider = true }: AnalyticsFiltersProps) {
  const { client, can } = useAuth()

  // Both selectors are populated from the API, and each is skipped when the viewer
  // cannot read that resource — a filter listing names it may not otherwise see would
  // leak exactly what the permission withholds.
  const apps = useQuery({
    queryKey: keys.apps.list({ page: 1, perPage: 100 }),
    queryFn: () => client.apps.list({ page: 1, perPage: 100 }),
    enabled: showApp && can(AdminPermissions.appsRead),
  })
  const providers = useQuery({
    queryKey: keys.providers.list({ page: 1, perPage: 100 }),
    queryFn: () => client.providers.list({ page: 1, perPage: 100 }),
    enabled: showProvider && can(AdminPermissions.providersRead),
  })

  function shiftDays(days: number) {
    const to = new Date()
    const from = new Date(to)
    from.setDate(from.getDate() - days)
    // Hourly buckets over 90 days would exceed what the server will return, so the
    // interval follows the range rather than being left to fail.
    onChange({ ...range, from: from.toISOString(), to: to.toISOString(), interval: days <= 2 ? "hour" : "day" })
  }

  return (
    <div className="flex flex-wrap items-end gap-3">
      <div className="flex flex-col gap-2">
        <Label htmlFor="range-preset">Range</Label>
        <Select
          value={String(Math.max(1, Math.round((Date.parse(range.to) - Date.parse(range.from)) / 86_400_000)))}
          onValueChange={(value) => value && shiftDays(Number(value))}
        >
          <SelectTrigger id="range-preset" className="w-40">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {ranges.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="flex flex-col gap-2">
        <Label htmlFor="range-from">From</Label>
        <Input
          id="range-from"
          type="date"
          className="w-40"
          value={asDateInput(range.from)}
          max={asDateInput(range.to)}
          onChange={(event) =>
            event.target.value && onChange({ ...range, from: new Date(event.target.value).toISOString() })
          }
        />
      </div>
      <div className="flex flex-col gap-2">
        <Label htmlFor="range-to">To</Label>
        <Input
          id="range-to"
          type="date"
          className="w-40"
          value={asDateInput(range.to)}
          min={asDateInput(range.from)}
          onChange={(event) =>
            event.target.value && onChange({ ...range, to: new Date(event.target.value).toISOString() })
          }
        />
      </div>

      {showApp && (
        <div className="flex flex-col gap-2">
          <Label htmlFor="range-app">App</Label>
          <Select
            value={range.appId ?? anyValue}
            onValueChange={(value) => onChange({ ...range, appId: value === anyValue ? undefined : (value ?? undefined) })}
          >
            <SelectTrigger id="range-app" className="w-48">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={anyValue}>All apps</SelectItem>
              {(apps.data?.items ?? []).map((app) => (
                <SelectItem key={app.id} value={app.id}>
                  {app.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}

      {showProvider && (
        <div className="flex flex-col gap-2">
          <Label htmlFor="range-provider">Provider</Label>
          <Select
            value={range.providerAccountId ?? anyValue}
            onValueChange={(value) =>
              onChange({ ...range, providerAccountId: value === anyValue ? undefined : (value ?? undefined) })
            }
          >
            <SelectTrigger id="range-provider" className="w-48">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={anyValue}>All providers</SelectItem>
              {(providers.data?.items ?? []).map((account) => (
                <SelectItem key={account.id} value={account.id}>
                  {account.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      )}
    </div>
  )
}
