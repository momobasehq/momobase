import { useState } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { AdminPermissions } from "@momobase/sdk";

import {
	ServiceMixChart,
	TransactionsChart,
} from "@/components/analytics-charts";
import {
	AnalyticsFilters,
	defaultRange,
	toQuery,
	type AnalyticsRange,
} from "@/components/analytics-filters";
import { Activity, CircleCheck, CircleX, Database, Plug } from "lucide-react";

import { StatusBadge } from "@/components/status-badge";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useAuth } from "@/hooks/use-auth";
import { formatDateTime } from "@/lib/format";
import { keys } from "@/lib/query-keys";

/** Tile renders one headline figure. */
function Tile({
	label,
	value,
	hint,
	icon: Icon,
	loading,
}: {
	label: string;
	value: string | number;
	hint?: string;
	icon: typeof Activity;
	loading?: boolean;
}) {
	return (
		<Card size="sm">
			<CardHeader>
				<CardDescription className="flex items-center gap-2">
					<Icon className="size-3.5" />
					{label}
				</CardDescription>
				<CardTitle className="text-2xl">
					{loading ? <Skeleton className="h-7 w-16" /> : value}
				</CardTitle>
			</CardHeader>
			{hint && (
				<CardContent className="text-muted-foreground">
					{hint}
				</CardContent>
			)}
		</Card>
	);
}

/** Overview is the landing screen: system health, traffic, and the configuration. */
export function Overview() {
	const { client, can } = useAuth();
	const health = useQuery({
		queryKey: keys.system.health(),
		queryFn: () => client.system.health(),
	});
	const info = useQuery({
		queryKey: keys.system.info(),
		queryFn: () => client.system.info(),
	});

	const [range, setRange] = useState<AnalyticsRange>(defaultRange);
	const showCharts = can(AdminPermissions.transactionsRead);
	const analytics = useQuery({
		queryKey: keys.analytics.transactions(toQuery(range)),
		queryFn: () => client.analytics.transactions(toQuery(range)),
		enabled: showCharts,
		placeholderData: keepPreviousData,
	});

	return (
		<div className="flex flex-col gap-4">
			<div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
				<Tile
					label="System"
					value={health.data?.ok ? "Healthy" : "Degraded"}
					hint={
						health.data
							? `Database ${health.data.database}`
							: undefined
					}
					icon={health.data?.ok ? CircleCheck : CircleX}
					loading={health.isPending}
				/>
				<Tile
					label="Runtime providers"
					value={health.data?.runtime_provider_count ?? 0}
					hint="Adapters loaded in memory"
					icon={Plug}
					loading={health.isPending}
				/>
				<Tile
					label="Active accounts"
					value={health.data?.active_provider_account_count ?? 0}
					hint="Provider accounts serving traffic"
					icon={Database}
					loading={health.isPending}
				/>
				<Tile
					label="Workers"
					value={health.data?.workers_configured.length ?? 0}
					hint={
						health.data?.workers_configured.join(", ") ||
						"None configured"
					}
					icon={Activity}
					loading={health.isPending}
				/>
			</div>

			{showCharts && (
				<div className="flex flex-col gap-4">
					{/* One filter row above both charts, so the two are always of the same window;
              per-chart ranges are how a dashboard ends up inviting a false comparison. */}
					<AnalyticsFilters range={range} onChange={setRange} />
					<div className="grid gap-4 xl:grid-cols-2">
						<TransactionsChart
							data={analytics.data}
							loading={analytics.isPending}
						/>
						<ServiceMixChart
							data={analytics.data}
							loading={analytics.isPending}
						/>
					</div>
				</div>
			)}

			<Card>
				<CardHeader>
					<CardTitle>Deployment</CardTitle>
					<CardDescription>
						What this binary is running.
					</CardDescription>
				</CardHeader>
				<CardContent>
					{info.isPending ? (
						<Skeleton className="h-24 w-full" />
					) : (
						<dl className="grid gap-x-8 gap-y-3 sm:grid-cols-2 lg:grid-cols-3">
							{[
								["Application", info.data?.app_name],
								["Environment", info.data?.app_env],
								["Database", info.data?.db_type],
								["Address", info.data?.addr],
								["Go version", info.data?.go_version],
								[
									"Server time",
									formatDateTime(info.data?.server_time),
								],
							].map(([label, value]) => (
								<div key={label}>
									<dt className="text-muted-foreground">
										{label}
									</dt>
									<dd className="font-medium">
										{value || "—"}
									</dd>
								</div>
							))}
							<div>
								<dt className="text-muted-foreground">
									Workers enabled
								</dt>
								<dd className="mt-1">
									<StatusBadge
										status={
											info.data?.workers_enabled
												? "active"
												: "inactive"
										}
									/>
								</dd>
							</div>
						</dl>
					)}
				</CardContent>
			</Card>
		</div>
	);
}
