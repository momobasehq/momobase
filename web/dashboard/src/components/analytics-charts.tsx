import { Area, AreaChart, CartesianGrid, Line, LineChart, XAxis, YAxis } from "recharts"
import type { AnalyticsBucket, TransactionAnalytics } from "@momobase/sdk"

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { ChartContainer, ChartLegend, ChartLegendContent, ChartTooltip, ChartTooltipContent, type ChartConfig } from "@/components/ui/chart"
import { Skeleton } from "@/components/ui/skeleton"

/** Shortens a bucket label for an axis and tooltip: the year is constant across a
 * chart's range. Typed loosely because Recharts hands the tooltip a ReactNode. */
function tick(value: unknown) {
  const period = String(value ?? "")
  // "2026-08-18" -> "Aug 18"; "2026-08-18 14:00" -> "14:00"
  if (period.includes(" ")) return period.split(" ")[1] ?? period
  const date = new Date(`${period}T00:00:00Z`)
  return Number.isNaN(date.getTime())
    ? period
    : date.toLocaleDateString(undefined, { month: "short", day: "numeric", timeZone: "UTC" })
}

/** Only every nth tick is drawn, so a 90-day range does not collide its own labels. */
function tickInterval(count: number) {
  return Math.max(0, Math.ceil(count / 12) - 1)
}

interface ChartProps {
  data?: TransactionAnalytics
  loading?: boolean
  /** Overrides the card title where the page already names the subject. */
  title?: string
  description?: string
}

function Empty({ loading }: { loading?: boolean }) {
  if (loading) return <Skeleton className="h-56 w-full" />
  return (
    <div className="text-muted-foreground flex h-56 items-center justify-center">No transactions in this range.</div>
  )
}

/** One series, so no legend: the card title names it. Area over line because a single
 * magnitude-over-time reads better filled, and the fill is the same hue at low alpha
 * rather than a second colour. */
const totalConfig = {
  total: { label: "Transactions", color: "var(--chart-1)" },
} satisfies ChartConfig

/** TransactionsChart shows how much traffic the range saw, bucket by bucket. */
export function TransactionsChart({ data, loading, title, description }: ChartProps) {
  const buckets = data?.buckets ?? []
  const empty = !loading && buckets.every((bucket) => bucket.total === 0)

  return (
    <Card>
      <CardHeader>
        <CardTitle>{title ?? "Transactions"}</CardTitle>
        <CardDescription>
          {description ?? "Every collection and disbursement recorded, whatever its outcome."}
          {data ? ` ${data.total.toLocaleString()} in range.` : ""}
        </CardDescription>
      </CardHeader>
      <CardContent>
        {loading || empty ? (
          <Empty loading={loading} />
        ) : (
          <ChartContainer config={totalConfig} className="h-56 w-full">
            <AreaChart data={buckets} margin={{ left: 4, right: 8, top: 8 }}>
              {/* Horizontal only, and recessive: the grid is a reading aid, not data. */}
              <CartesianGrid vertical={false} strokeOpacity={0.4} />
              <XAxis
                dataKey="period"
                tickFormatter={tick}
                interval={tickInterval(buckets.length)}
                tickLine={false}
                axisLine={false}
                tickMargin={8}
              />
              <YAxis width={36} allowDecimals={false} tickLine={false} axisLine={false} />
              <ChartTooltip content={<ChartTooltipContent labelFormatter={tick} />} />
              <Area
                dataKey="total"
                type="monotone"
                stroke="var(--color-total)"
                strokeWidth={2}
                fill="var(--color-total)"
                fillOpacity={0.15}
              />
            </AreaChart>
          </ChartContainer>
        )}
      </CardContent>
    </Card>
  )
}

/** Two series with distinct identity, so a legend is always present and each line is
 * its own validated hue rather than two steps of one. */
const mixConfig = {
  collection: { label: "Collections", color: "var(--chart-1)" },
  disbursement: { label: "Disbursements", color: "var(--chart-2)" },
} satisfies ChartConfig

/** Flattens the nested service counts, which Recharts cannot address directly. */
function mixSeries(buckets: AnalyticsBucket[]) {
  return buckets.map((bucket) => ({
    period: bucket.period,
    collection: bucket.by_service.collection,
    disbursement: bucket.by_service.disbursement,
  }))
}

/** ServiceMixChart compares the two directions money moves over the same range. */
export function ServiceMixChart({ data, loading, title, description }: ChartProps) {
  const series = mixSeries(data?.buckets ?? [])
  const empty = !loading && series.every((point) => point.collection === 0 && point.disbursement === 0)

  return (
    <Card>
      <CardHeader>
        <CardTitle>{title ?? "Collections vs disbursements"}</CardTitle>
        <CardDescription>
          {description ?? "Money in against money out, as transaction counts."}
          {data ? ` ${data.by_service.collection.toLocaleString()} in, ${data.by_service.disbursement.toLocaleString()} out.` : ""}
        </CardDescription>
      </CardHeader>
      <CardContent>
        {loading || empty ? (
          <Empty loading={loading} />
        ) : (
          <ChartContainer config={mixConfig} className="h-56 w-full">
            <LineChart data={series} margin={{ left: 4, right: 8, top: 8 }}>
              <CartesianGrid vertical={false} strokeOpacity={0.4} />
              <XAxis
                dataKey="period"
                tickFormatter={tick}
                interval={tickInterval(series.length)}
                tickLine={false}
                axisLine={false}
                tickMargin={8}
              />
              <YAxis width={36} allowDecimals={false} tickLine={false} axisLine={false} />
              <ChartTooltip content={<ChartTooltipContent labelFormatter={tick} />} />
              {/* Counts share one scale, so both series sit on one axis. A second y-axis
                  would let the two be scaled independently and imply a crossover that
                  the numbers do not contain. */}
              <Line dataKey="collection" type="monotone" stroke="var(--color-collection)" strokeWidth={2} dot={false} />
              <Line dataKey="disbursement" type="monotone" stroke="var(--color-disbursement)" strokeWidth={2} dot={false} />
              <ChartLegend content={<ChartLegendContent />} />
            </LineChart>
          </ChartContainer>
        )}
      </CardContent>
    </Card>
  )
}
