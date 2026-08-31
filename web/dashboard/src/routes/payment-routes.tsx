import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router";
import { toast } from "sonner";
import {
	AdminPermissions,
	type PaymentMethod,
	type PaymentRoute,
	type ServiceType,
} from "momobase";

import { DataTable, type Column } from "@/components/data-table";
import { GuardedAction } from "@/components/guarded-action";
import { PaginationControls } from "@/components/pagination-controls";
import { StatusBadge } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "@/components/ui/card";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "@/components/ui/select";
import { useAuth } from "@/hooks/use-auth";
import { usePagedQuery } from "@/hooks/use-paged-query";
import { keys } from "@/lib/query-keys";
import { titleCase } from "@/lib/format";

/** CreateRouteDialog connects a provider account to a service and payment method. */
function CreateRouteDialog({
	open,
	onOpenChange,
}: {
	open: boolean;
	onOpenChange: (open: boolean) => void;
}) {
	const { client } = useAuth();
	const queryClient = useQueryClient();
	const [form, setForm] = useState({
		service_type: "collection" as ServiceType,
		payment_method: "" as PaymentMethod | "",
		provider_account_id: "",
		priority: 1,
		active: true,
	});

	const accounts = useQuery({
		queryKey: keys.providers.list({ page: 1, perPage: 100 }),
		queryFn: () => client.providers.list({ page: 1, perPage: 100 }),
	});
	const runtimes = useQuery({
		queryKey: keys.system.runtime({ page: 1, perPage: 100 }),
		queryFn: () =>
			client.system.runtimeProviders({ page: 1, perPage: 100 }),
	});
	const runtime = runtimes.data?.items.find(
		(item) => item.provider_account_id === form.provider_account_id,
	);
	const methods =
		runtime?.capabilities
			.filter(
				(capability) => capability.service_type === form.service_type,
			)
			.map((capability) => capability.payment_method) ?? [];

	const create = useMutation({
		mutationFn: () =>
			client.routes.create({
				...form,
				payment_method: form.payment_method as PaymentMethod,
			}),
		onSuccess: async () => {
			toast.success("Route created");
			onOpenChange(false);
			await queryClient.invalidateQueries({ queryKey: keys.routes.all });
		},
		onError: (error: Error) => toast.error(error.message),
	});

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>New route</DialogTitle>
					<DialogDescription>
						The lowest-priority active route whose account is
						eligible wins.
					</DialogDescription>
				</DialogHeader>
				<div className="flex flex-col gap-3">
					<div className="flex flex-col gap-2">
						<Label htmlFor="route-service">Service</Label>
						<Select
							value={form.service_type}
							onValueChange={(value) =>
								setForm({
									...form,
									service_type:
										(value as ServiceType) ?? "collection",
									payment_method: "",
								})
							}
						>
							<SelectTrigger id="route-service">
								<SelectValue />
							</SelectTrigger>
							<SelectContent>
								<SelectItem value="collection">
									Collection
								</SelectItem>
								<SelectItem value="disbursement">
									Disbursement
								</SelectItem>
							</SelectContent>
						</Select>
					</div>
					<div className="flex flex-col gap-2">
						<Label htmlFor="route-account">Provider account</Label>
						<Select
							value={form.provider_account_id}
							onValueChange={(id) =>
								setForm({
									...form,
									provider_account_id: id ?? "",
									payment_method: "",
								})
							}
						>
							<SelectTrigger id="route-account">
								<SelectValue
									placeholder={
										accounts.isPending
											? "Loading…"
											: "Select an account"
									}
								/>
							</SelectTrigger>
							<SelectContent>
								{(accounts.data?.items ?? [])
									.filter((account) => account.active)
									.map((account) => (
										<SelectItem
											key={account.id}
											value={account.id}
										>
											{account.name} (
											{account.provider_code})
										</SelectItem>
									))}
							</SelectContent>
						</Select>
					</div>
					<div className="flex flex-col gap-2">
						<Label htmlFor="route-method">Payment method</Label>
						<Select
							value={form.payment_method}
							onValueChange={(paymentMethod) =>
								setForm({
									...form,
									payment_method:
										paymentMethod as PaymentMethod,
								})
							}
							disabled={!form.provider_account_id}
						>
							<SelectTrigger id="route-method">
								<SelectValue placeholder="Select a supported method" />
							</SelectTrigger>
							<SelectContent>
								{methods.map((method) => (
									<SelectItem key={method} value={method}>
										{titleCase(method)}
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
							onChange={(event) =>
								setForm({
									...form,
									priority: Number(event.target.value) || 1,
								})
							}
						/>
					</div>
				</div>
				<DialogFooter>
					<Button
						variant="outline"
						onClick={() => onOpenChange(false)}
					>
						Cancel
					</Button>
					<Button
						onClick={() => create.mutate()}
						disabled={
							create.isPending ||
							!form.payment_method ||
							!form.provider_account_id
						}
					>
						Create
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}

/** PaymentRoutes manages the routing table. */
export function PaymentRoutes() {
	const { client } = useAuth();
	const queryClient = useQueryClient();
	const paged = usePagedQuery(keys.routes.list, (page) =>
		client.routes.list(page),
	);
	const [creating, setCreating] = useState(false);

	const update = useMutation({
		mutationFn: ({
			id,
			priority,
			active,
		}: {
			id: string;
			priority: number;
			active: boolean;
		}) => client.routes.update(id, { priority, active }),
		onSuccess: async () => {
			toast.success("Route updated");
			await queryClient.invalidateQueries({ queryKey: keys.routes.all });
		},
		onError: (error: Error) => toast.error(error.message),
	});

	const columns: Column<PaymentRoute>[] = [
		{
			key: "service",
			header: "Service",
			cell: (route) => titleCase(route.service_type),
		},
		{
			key: "method",
			header: "Method",
			cell: (route) => (
				<code className="font-medium">{route.payment_method}</code>
			),
		},
		{
			key: "account",
			header: "Provider account",
			cell: (route) => (
				<Link
					className="font-medium underline-offset-4 hover:underline"
					to={`/providers/${route.provider_account_id}`}
				>
					{route.provider_name || route.provider_account_id}
				</Link>
			),
		},
		{
			key: "priority",
			header: "Priority",
			align: "end",
			cell: (route) => route.priority,
		},
		{
			key: "active",
			header: "Status",
			cell: (route) => (
				<StatusBadge status={route.active ? "active" : "inactive"} />
			),
		},
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
					onClick={() =>
						update.mutate({
							id: route.id,
							priority: route.priority,
							active: !route.active,
						})
					}
				>
					{route.active ? "Disable" : "Enable"}
				</GuardedAction>
			),
		},
	];

	return (
		<Card>
			<CardHeader>
				<CardTitle>Routes</CardTitle>
				<CardDescription>
					Which provider account serves each service and payment
					method.
				</CardDescription>
				<div className="ms-auto">
					<GuardedAction
						permission={AdminPermissions.routesCreate}
						size="sm"
						onClick={() => setCreating(true)}
					>
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
			<CreateRouteDialog open={creating} onOpenChange={setCreating} />
		</Card>
	);
}
