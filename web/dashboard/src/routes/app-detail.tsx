import { useState } from "react";
import {
	keepPreviousData,
	useMutation,
	useQuery,
	useQueryClient,
} from "@tanstack/react-query";
import { Link, useParams } from "react-router";
import { ArrowLeft, Copy, TriangleAlert } from "lucide-react";
import { toast } from "sonner";

import { DataTable, type Column } from "@/components/data-table";
import { ChargeFields, zeroCharges } from "@/components/charge-fields";
import { ServiceMixChart } from "@/components/analytics-charts";
import {
	AnalyticsFilters,
	defaultRange,
	toQuery,
	type AnalyticsRange,
} from "@/components/analytics-filters";
import { AppTester } from "@/components/app-tester";
import { GuardedAction } from "@/components/guarded-action";
import { PaginationControls } from "@/components/pagination-controls";
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
import { Checkbox } from "@/components/ui/checkbox";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import {
	AdminPermissions,
	type AppCredential,
	type ChargeRule,
	type CreatedCredential,
} from "momobase";

import { useAuth } from "@/hooks/use-auth";
import { usePagedQuery } from "@/hooks/use-paged-query";
import { formatDateTime } from "@/lib/format";
import { keys } from "@/lib/query-keys";

/**
 * SecretDialog shows a client secret exactly once.
 *
 * The API returns the plaintext only on creation and rotation — it is stored hashed —
 * so this is the single moment it can be copied. Closing the dialog loses it for good,
 * which the copy is explicit about rather than leaving the operator to find out later.
 */
function SecretDialog({
	created,
	onClose,
}: {
	created?: CreatedCredential;
	onClose: () => void;
}) {
	async function copy() {
		if (!created) return;
		try {
			await navigator.clipboard.writeText(created.client_secret);
			toast.success("Client secret copied");
		} catch {
			toast.error(
				"Copying failed — select the value and copy it manually",
			);
		}
	}

	return (
		<Dialog
			open={Boolean(created)}
			onOpenChange={(open) => !open && onClose()}
		>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>Client secret</DialogTitle>
					<DialogDescription>
						Copy it now — it is stored hashed and cannot be shown
						again.
					</DialogDescription>
				</DialogHeader>
				<Alert>
					<TriangleAlert />
					<AlertTitle>Shown once</AlertTitle>
					<AlertDescription>
						Closing this dialog discards the secret. Rotate the
						credential to issue a new one.
					</AlertDescription>
				</Alert>
				<div className="flex flex-col gap-2">
					<Label htmlFor="client-id">Client ID</Label>
					<Input
						id="client-id"
						readOnly
						value={created?.credential.client_id ?? ""}
					/>
					<Label htmlFor="client-secret">Client secret</Label>
					<div className="flex gap-2">
						<Input
							id="client-secret"
							readOnly
							value={created?.client_secret ?? ""}
							className="font-mono"
						/>
						<Button
							variant="outline"
							size="icon"
							onClick={() => void copy()}
							aria-label="Copy client secret"
						>
							<Copy />
						</Button>
					</div>
				</div>
				<DialogFooter>
					<Button onClick={onClose}>Done</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}

/** AppDetail shows one app and manages its API credentials. */
export function AppDetail() {
	const { appId = "" } = useParams();
	const { client, can } = useAuth();
	const queryClient = useQueryClient();
	const [created, setCreated] = useState<CreatedCredential>();
	const [naming, setNaming] = useState(false);
	const [name, setName] = useState("");
	const [editing, setEditing] = useState(false);
	const [details, setDetails] = useState({
		name: "",
		description: "",
		currency: "UGX",
		charges: zeroCharges,
	});
	const [scopes, setScopes] = useState<string[]>([]);

	// Scopes come from the server's app-audience catalogue, and the server validates
	// against the same list — so a scope that cannot work is no longer typeable.
	const catalogue = useQuery({
		queryKey: keys.authz.permissions("app"),
		queryFn: () => client.authz.permissions("app"),
		enabled: naming,
	});

	// Credentials are their own permission, so the card is hidden for a role that may
	// read apps but not their credentials.
	const showCredentials = can(AdminPermissions.credentialsRead);
	const [range, setRange] = useState<AnalyticsRange>(defaultRange);
	const showAnalytics = can(AdminPermissions.transactionsRead);
	const analytics = useQuery({
		queryKey: keys.analytics.transactions({ ...toQuery(range), appId }),
		queryFn: () =>
			client.analytics.transactions({ ...toQuery(range), appId }),
		enabled: showAnalytics && Boolean(appId),
		placeholderData: keepPreviousData,
	});

	const app = useQuery({
		queryKey: keys.apps.detail(appId),
		queryFn: () => client.apps.get(appId),
		enabled: Boolean(appId),
	});
	const paged = usePagedQuery(
		(page) => keys.apps.credentials(appId, page),
		(page) => client.apps.credentials(appId, page),
		20,
		showCredentials && Boolean(appId),
	);

	async function refreshCredentials() {
		await queryClient.invalidateQueries({ queryKey: keys.apps.all });
	}

	const changeStatus = useMutation({
		mutationFn: (status: "active" | "disabled" | "suspended") =>
			client.apps.changeStatus(appId, status),
		onSuccess: async () => {
			toast.success("Status updated");
			await queryClient.invalidateQueries({ queryKey: keys.apps.all });
		},
		onError: (error: Error) => toast.error(error.message),
	});

	const save = useMutation({
		mutationFn: () => client.apps.update(appId, details),
		onSuccess: async () => {
			toast.success("App updated");
			setEditing(false);
			await refreshCredentials();
		},
		onError: (error: Error) => toast.error(error.message),
	});

	const create = useMutation({
		mutationFn: () =>
			client.apps.createCredential(appId, {
				name: name || undefined,
				scopes: scopes.length ? scopes.join(" ") : undefined,
			}),
		onSuccess: async (result) => {
			setNaming(false);
			setName("");
			setScopes([]);
			setCreated(result);
			await refreshCredentials();
		},
		onError: (error: Error) => toast.error(error.message),
	});

	const rotate = useMutation({
		mutationFn: (credentialId: string) =>
			client.apps.rotateCredential(appId, credentialId),
		onSuccess: async (result) => {
			setCreated(result);
			await refreshCredentials();
		},
		onError: (error: Error) => toast.error(error.message),
	});

	const revoke = useMutation({
		mutationFn: (credentialId: string) =>
			client.apps.revokeCredential(appId, credentialId),
		onSuccess: async () => {
			toast.success("Credential revoked");
			await refreshCredentials();
		},
		onError: (error: Error) => toast.error(error.message),
	});

	const columns: Column<AppCredential>[] = [
		{
			key: "name",
			header: "Name",
			cell: (credential) => credential.name || "—",
		},
		{
			key: "client_id",
			header: "Client ID",
			cell: (credential) => <code>{credential.client_id}</code>,
		},
		{
			key: "scopes",
			header: "Scopes",
			cell: (credential) => <code>{credential.scopes || "—"}</code>,
		},
		{
			key: "status",
			header: "Status",
			cell: (credential) => <StatusBadge status={credential.status} />,
		},
		{
			key: "used",
			header: "Last used",
			cell: (credential) => formatDateTime(credential.last_used_at),
		},
		{
			key: "actions",
			header: "",
			align: "end",
			cell: (credential) => (
				<div className="flex justify-end gap-2">
					<GuardedAction
						permission={AdminPermissions.credentialsUpdate}
						variant="outline"
						size="sm"
						disabled={
							rotate.isPending || credential.status !== "active"
						}
						onClick={() => rotate.mutate(credential.id)}
					>
						Rotate
					</GuardedAction>
					<GuardedAction
						permission={AdminPermissions.credentialsUpdate}
						variant="destructive"
						size="sm"
						disabled={
							revoke.isPending || credential.status !== "active"
						}
						onClick={() => revoke.mutate(credential.id)}
					>
						Revoke
					</GuardedAction>
				</div>
			),
		},
	];

	return (
		<div className="flex flex-col gap-4">
			<Button
				variant="ghost"
				size="sm"
				render={<Link to="/apps" />}
				className="self-start"
			>
				<ArrowLeft />
				All apps
			</Button>

			<Card>
				<CardHeader>
					<CardTitle>
						{app.data?.name ?? <Skeleton className="h-5 w-40" />}
					</CardTitle>
					<CardDescription>
						{app.data?.description || "No description."}
					</CardDescription>
					<div className="ms-auto">
						<GuardedAction
							permission={AdminPermissions.appsUpdate}
							variant="outline"
							size="sm"
							onClick={() => {
								setDetails({
									name: app.data?.name ?? "",
									description: app.data?.description ?? "",
									currency: app.data?.currency ?? "UGX",
									charges: app.data?.charges ?? zeroCharges,
								});
								setEditing(true);
							}}
						>
							Edit
						</GuardedAction>
					</div>
				</CardHeader>
				<CardContent className="flex flex-wrap items-end gap-6">
					<div>
						<p className="text-muted-foreground">Environment</p>
						<StatusBadge status={app.data?.environment} />
					</div>
					<div>
						<p className="text-muted-foreground">Status</p>
						<StatusBadge status={app.data?.status} />
					</div>
					<div>
						<p className="text-muted-foreground">Currency</p>
						<code>{app.data?.currency ?? "—"}</code>
					</div>
					<div>
						<p className="text-muted-foreground">Collection fee</p>
						<span>
							{formatCharge(app.data?.charges.collection)}
						</span>
					</div>
					<div>
						<p className="text-muted-foreground">
							Disbursement fee
						</p>
						<span>
							{formatCharge(app.data?.charges.disbursement)}
						</span>
					</div>
					<div>
						<p className="text-muted-foreground">App ID</p>
						<code>{appId}</code>
					</div>
					{/* Suspended is distinct from disabled on the API, so both are offered rather
              than collapsed into one toggle that cannot express the difference. */}
					<div className="ms-auto flex gap-2">
						{app.data?.status === "active" ? (
							<>
								<GuardedAction
									permission={AdminPermissions.appsUpdate}
									variant="outline"
									size="sm"
									disabled={changeStatus.isPending}
									onClick={() =>
										changeStatus.mutate("suspended")
									}
								>
									Suspend
								</GuardedAction>
								<GuardedAction
									permission={AdminPermissions.appsUpdate}
									variant="destructive"
									size="sm"
									disabled={changeStatus.isPending}
									onClick={() =>
										changeStatus.mutate("disabled")
									}
								>
									Disable
								</GuardedAction>
							</>
						) : (
							<GuardedAction
								permission={AdminPermissions.appsUpdate}
								size="sm"
								disabled={changeStatus.isPending || !app.data}
								onClick={() => changeStatus.mutate("active")}
							>
								Activate
							</GuardedAction>
						)}
					</div>
				</CardContent>
			</Card>

			{/* Tabs rather than a stack: the page now carries four unrelated concerns, and
          scrolling past credentials to reach the tester made both harder to find. */}
			<Tabs defaultValue="credentials">
				<TabsList>
					<TabsTrigger value="credentials">Credentials</TabsTrigger>
					<TabsTrigger value="analytics">Analytics</TabsTrigger>
					<TabsTrigger value="testing">Testing</TabsTrigger>
				</TabsList>

				<TabsContent value="credentials" className="pt-3">
					{showCredentials && (
						<Card>
							<CardHeader>
								<CardTitle>Credentials</CardTitle>
								<CardDescription>
									Client credentials this app uses to obtain
									access tokens.
								</CardDescription>
								<div className="ms-auto">
									<GuardedAction
										permission={
											AdminPermissions.credentialsCreate
										}
										size="sm"
										onClick={() => setNaming(true)}
									>
										New credential
									</GuardedAction>
								</div>
							</CardHeader>
							<CardContent>
								<DataTable
									columns={columns}
									rows={paged.items}
									rowKey={(credential) => credential.id}
									loading={paged.loading}
									empty="This app has no credentials yet."
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
					)}
				</TabsContent>

				<TabsContent
					value="analytics"
					className="flex flex-col gap-4 pt-3"
				>
					{showAnalytics ? (
						<>
							{/* The app is fixed by the page, so its selector is suppressed; range and
                  provider still apply. */}
							<AnalyticsFilters
								range={range}
								onChange={setRange}
								showApp={false}
							/>
							<ServiceMixChart
								data={analytics.data}
								loading={analytics.isPending}
								title="This app's payments"
								description="Collections and disbursements created by this app alone."
							/>
						</>
					) : (
						<p className="text-muted-foreground">
							Requires the transactions:read permission.
						</p>
					)}
				</TabsContent>

				<TabsContent value="testing" className="pt-3">
					{can(AdminPermissions.appsTest) ? (
						<AppTester
							clientId={created?.credential.client_id}
							clientSecret={created?.client_secret}
						/>
					) : (
						<p className="text-muted-foreground">
							Requires the apps:test permission.
						</p>
					)}
				</TabsContent>
			</Tabs>

			<Dialog open={naming} onOpenChange={setNaming}>
				<DialogContent>
					<DialogHeader>
						<DialogTitle>New credential</DialogTitle>
						<DialogDescription>
							The client secret is shown once, immediately after
							creation.
						</DialogDescription>
					</DialogHeader>
					<div className="flex flex-col gap-2">
						<Label htmlFor="credential-name">Name</Label>
						<Input
							id="credential-name"
							value={name}
							onChange={(event) => setName(event.target.value)}
							placeholder="Checkout service"
						/>
					</div>
					<div className="flex flex-col gap-2">
						<Label>Scopes</Label>
						{catalogue.isPending ? (
							<Skeleton className="h-20 w-full" />
						) : (
							(catalogue.data?.items ?? []).map((permission) => (
								<label
									key={permission.id}
									className="flex items-start gap-2"
								>
									<Checkbox
										checked={scopes.includes(
											permission.code,
										)}
										onCheckedChange={(checked) =>
											setScopes((current) =>
												checked === true
													? [
															...current,
															permission.code,
														]
													: current.filter(
															(held) =>
																held !==
																permission.code,
														),
											)
										}
									/>
									<span>
										<code>{permission.code}</code>
										<span className="text-muted-foreground">
											{" "}
											— {permission.description}
										</span>
									</span>
								</label>
							))
						)}
						<p className="text-muted-foreground">
							Leaving all unselected issues the server's default
							scopes.
						</p>
					</div>
					<DialogFooter>
						<Button
							variant="outline"
							onClick={() => setNaming(false)}
						>
							Cancel
						</Button>
						<Button
							onClick={() => create.mutate()}
							disabled={create.isPending}
						>
							Create
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

			<Dialog open={editing} onOpenChange={setEditing}>
				<DialogContent>
					<DialogHeader>
						<DialogTitle>Edit app</DialogTitle>
						<DialogDescription>
							Changes apply only to future transactions.
						</DialogDescription>
					</DialogHeader>
					<div className="flex flex-col gap-3">
						<div className="flex flex-col gap-2">
							<Label htmlFor="edit-app-name">Name</Label>
							<Input
								id="edit-app-name"
								value={details.name}
								onChange={(event) =>
									setDetails({
										...details,
										name: event.target.value,
									})
								}
							/>
						</div>
						<div className="flex flex-col gap-2">
							<Label htmlFor="edit-app-currency">Currency</Label>
							<Input
								id="edit-app-currency"
								maxLength={3}
								value={details.currency}
								onChange={(event) =>
									setDetails({
										...details,
										currency:
											event.target.value.toUpperCase(),
									})
								}
							/>
						</div>
						<ChargeFields
							id="edit-app-charges"
							value={details.charges}
							onChange={(charges) =>
								setDetails({ ...details, charges })
							}
						/>
						<div className="flex flex-col gap-2">
							<Label htmlFor="edit-app-description">
								Description
							</Label>
							<Textarea
								id="edit-app-description"
								value={details.description}
								onChange={(event) =>
									setDetails({
										...details,
										description: event.target.value,
									})
								}
							/>
						</div>
					</div>
					<DialogFooter>
						<Button
							variant="outline"
							onClick={() => setEditing(false)}
						>
							Cancel
						</Button>
						<Button
							onClick={() => save.mutate()}
							disabled={
								save.isPending ||
								!details.name ||
								details.currency.length !== 3
							}
						>
							Save
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

			<SecretDialog
				created={created}
				onClose={() => setCreated(undefined)}
			/>
		</div>
	);
}

function formatCharge(rule?: ChargeRule) {
	if (!rule) return "—";
	return rule.type === "percentage"
		? `${rule.value / 100}%`
		: `${rule.value} minor units`;
}
