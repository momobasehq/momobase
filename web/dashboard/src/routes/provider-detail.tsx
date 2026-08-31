import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { ArrowLeft } from "lucide-react";
import { useState, type ReactNode } from "react";
import { Link, useParams } from "react-router";
import { AdminPermissions, type ChargeRule } from "momobase";

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
import { StatusBadge } from "@/components/status-badge";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
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

function formatCharge(rule?: ChargeRule) {
	if (!rule) return "—";
	return rule.type === "percentage"
		? `${rule.value / 100}%`
		: `${rule.value} minor units`;
}

/** ProviderDetail shows one account's configuration metadata and payment activity. */
export function ProviderDetail() {
	const { providerId = "" } = useParams();
	const { client, can } = useAuth();
	const [range, setRange] = useState<AnalyticsRange>(() => ({
		...defaultRange(),
		providerAccountId: providerId,
	}));

	const account = useQuery({
		queryKey: keys.providers.detail(providerId),
		queryFn: () => client.providers.get(providerId),
		enabled: Boolean(providerId),
	});
	const showAnalytics = can(AdminPermissions.transactionsRead);
	const analyticsQuery = { ...toQuery(range), providerAccountId: providerId };
	const analytics = useQuery({
		queryKey: keys.analytics.transactions(analyticsQuery),
		queryFn: () => client.analytics.transactions(analyticsQuery),
		enabled: showAnalytics && account.isSuccess,
		placeholderData: keepPreviousData,
	});

	return (
		<div className="flex flex-col gap-4">
			<Button
				variant="ghost"
				size="sm"
				render={<Link to="/providers" />}
				className="self-start"
			>
				<ArrowLeft />
				All provider accounts
			</Button>

			{account.isError && (
				<Alert variant="destructive">
					<AlertTitle>Provider account unavailable</AlertTitle>
					<AlertDescription>{account.error.message}</AlertDescription>
				</Alert>
			)}

			{!account.isError && (
				<Card>
					<CardHeader>
						<CardTitle>
							{account.data?.name ?? (
								<Skeleton className="h-5 w-40" />
							)}
						</CardTitle>
						<CardDescription>
							{account.data
								? `${account.data.provider_code} provider account`
								: "Loading provider account details…"}
						</CardDescription>
					</CardHeader>
					{account.data && (
						<CardContent className="grid gap-x-8 gap-y-5 sm:grid-cols-2 lg:grid-cols-4">
							<Detail label="Status">
								<StatusBadge
									status={
										account.data.active
											? "active"
											: "inactive"
									}
								/>
							</Detail>
							<Detail label="Environment">
								<StatusBadge
									status={account.data.environment}
								/>
							</Detail>
							<Detail label="Provider">
								<code>{account.data.provider_code}</code>
							</Detail>
							<Detail label="Config version">
								v{account.data.config_version}
							</Detail>
							<Detail label="Country">
								<code>{account.data.country}</code>
							</Detail>
							<Detail label="Currency">
								<code>{account.data.currency}</code>
							</Detail>
							<Detail label="Collection fee">
								{formatCharge(account.data.charges.collection)}
							</Detail>
							<Detail label="Disbursement fee">
								{formatCharge(
									account.data.charges.disbursement,
								)}
							</Detail>
							<Detail label="Account ID">
								<code className="break-all">{providerId}</code>
							</Detail>
							<Detail label="Configuration hash">
								<code className="break-all">
									{account.data.config_hash || "—"}
								</code>
							</Detail>
							<Detail label="Created">
								{formatDateTime(account.data.created_at)}
							</Detail>
							<Detail label="Last updated">
								{formatDateTime(account.data.updated_at)}
							</Detail>
						</CardContent>
					)}
				</Card>
			)}

			{showAnalytics ? (
				!account.isError && (
					<div className="flex flex-col gap-4">
						<AnalyticsFilters
							range={range}
							onChange={setRange}
							showProvider={false}
						/>
						<div className="grid gap-4 xl:grid-cols-2">
							<TransactionsChart
								data={analytics.data}
								loading={analytics.isPending}
								title="Provider transactions"
								description="Payments routed through this provider account."
							/>
							<ServiceMixChart
								data={analytics.data}
								loading={analytics.isPending}
								title="Collections vs disbursements"
								description="Transaction counts handled by this provider account."
							/>
						</div>
					</div>
				)
			) : (
				<p className="text-muted-foreground">
					Provider analytics requires the transactions:read
					permission.
				</p>
			)}
		</div>
	);
}

function Detail({ label, children }: { label: string; children: ReactNode }) {
	return (
		<div className="min-w-0">
			<p className="text-muted-foreground">{label}</p>
			<div className="mt-1">{children}</div>
		</div>
	);
}
