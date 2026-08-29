import { useMemo, useState } from "react";
import { KeyRound, Loader2, Search, Send } from "lucide-react";
import {
	MomobaseClient,
	type AvailablePaymentMethod,
	type PaymentMethod,
	type ServiceType,
} from "@momobase/sdk";

import { Button } from "@/components/ui/button";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "@/components/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

/** A stable-ish reference, so repeated tests do not collide on the app's unique index. */
function reference(prefix: string) {
	return `${prefix}-${Date.now().toString(36).toUpperCase()}`;
}

interface AppTesterProps {
	/** Prefills the client ID when a credential was just issued on this page. */
	clientId?: string;
	/** Prefills the secret, which is only ever available immediately after issuing. */
	clientSecret?: string;
}

/**
 * AppTester drives the **public** API as the application, using credentials the operator
 * supplies rather than the admin session.
 *
 * That separation is the point: it exercises the same client an integrator uses, over the
 * same token grant and the same scopes, so a payment that works here works for them. The
 * transactions it creates are real, which is why the card is gated on apps:test rather
 * than being available to anyone who can read an app.
 */
export function AppTester({ clientId, clientSecret }: AppTesterProps) {
	const [credentials, setCredentials] = useState({
		clientId: clientId ?? "",
		clientSecret: clientSecret ?? "",
	});
	const [prefilled, setPrefilled] = useState<string>();
	const [methods, setMethods] = useState<AvailablePaymentMethod[]>([]);
	const [busy, setBusy] = useState<string>();
	const [result, setResult] = useState<{ label: string; body: unknown }>();
	const [error, setError] = useState<string>();
	const [lookup, setLookup] = useState({ id: "", reference: "" });
	const [form, setForm] = useState({
		payment_method: "" as PaymentMethod | "",
		scheme: "",
		account: "256770000000",
		amount: 5000,
		currency: "UGX",
		country: "UG",
	});

	// A freshly issued secret is only in memory for a moment, so adopt it the instant it
	// appears rather than making the operator copy it back in by hand.
	if (clientId && prefilled !== clientId) {
		setPrefilled(clientId);
		setCredentials({ clientId, clientSecret: clientSecret ?? "" });
	}

	// Same origin by default, since the dashboard is normally served by the very binary
	// it is testing. VITE_API_URL overrides it for a separately deployed frontend, which
	// then needs its origin in the server's CORS_ALLOWED_ORIGINS.
	const client = useMemo(
		() =>
			credentials.clientId && credentials.clientSecret
				? new MomobaseClient({
						baseUrl:
							import.meta.env.VITE_API_URL ||
							window.location.origin,
						clientId: credentials.clientId,
						clientSecret: credentials.clientSecret,
					})
				: undefined,
		[credentials.clientId, credentials.clientSecret],
	);

	async function run(
		label: string,
		action: (client: MomobaseClient) => Promise<unknown>,
	) {
		if (!client) {
			setError("Enter the app's client ID and secret first");
			return;
		}
		setBusy(label);
		setError(undefined);
		try {
			setResult({ label, body: await action(client) });
		} catch (cause) {
			setError(
				cause instanceof Error ? cause.message : `${label} failed`,
			);
			setResult(undefined);
		} finally {
			setBusy(undefined);
		}
	}

	async function pay(service: ServiceType) {
		const own = reference(service === "collection" ? "COLL" : "DISB");
		const payload = {
			payment_method: form.payment_method as PaymentMethod,
			scheme: form.scheme || undefined,
			account: form.account,
			amount: Number(form.amount),
			currency: form.currency,
			country: form.country,
			reference: own,
			description: `Dashboard test ${service}`,
		};
		await run(
			service === "collection" ? "Collection" : "Disbursement",
			async (app) => {
				const created =
					service === "collection"
						? await app.collections.create(
								{
									...payload,
									customer: { name: "Dashboard test" },
								},
								{ idempotencyKey: own },
							)
						: await app.disbursements.create(
								{
									...payload,
									recipient: { name: "Dashboard test" },
								},
								{ idempotencyKey: own },
							);
				// Carry the identifiers straight into the lookup tab; copying them by hand is the
				// step that makes a tester tedious.
				setLookup({
					id: created.transaction_id,
					reference: created.reference,
				});
				return created;
			},
		);
	}

	const ready = Boolean(client);
	const supports = (service: ServiceType) =>
		methods.some(
			(method) =>
				method.service_type === service &&
				method.payment_method === form.payment_method,
		);

	return (
		<Card>
			<CardHeader>
				<CardTitle>Test this app</CardTitle>
				<CardDescription>
					Runs the public API as the application, over the same token
					grant and scopes an integrator uses. Payments created here
					are real.
				</CardDescription>
			</CardHeader>
			<CardContent className="flex flex-col gap-4">
				<div className="grid gap-3 sm:grid-cols-2">
					<div className="flex flex-col gap-2">
						<Label htmlFor="tester-client-id">Client ID</Label>
						<Input
							id="tester-client-id"
							className="font-mono"
							value={credentials.clientId}
							onChange={(event) =>
								setCredentials({
									...credentials,
									clientId: event.target.value,
								})
							}
						/>
					</div>
					<div className="flex flex-col gap-2">
						<Label htmlFor="tester-client-secret">
							Client secret
						</Label>
						<Input
							id="tester-client-secret"
							type="password"
							className="font-mono"
							value={credentials.clientSecret}
							onChange={(event) =>
								setCredentials({
									...credentials,
									clientSecret: event.target.value,
								})
							}
							placeholder="Shown once when issued or rotated"
						/>
					</div>
				</div>

				<Tabs defaultValue="auth">
					<TabsList>
						<TabsTrigger value="auth">Token</TabsTrigger>
						<TabsTrigger value="pay">Payments</TabsTrigger>
						<TabsTrigger value="lookup">Lookup</TabsTrigger>
					</TabsList>

					<TabsContent
						value="auth"
						className="flex flex-col gap-3 pt-3"
					>
						<p className="text-muted-foreground">
							Exchanges the credentials for an access token, which
							is the first thing an integration does and the
							quickest way to tell a bad secret from a bad scope.
						</p>
						<div className="flex flex-wrap gap-2">
							<Button
								disabled={!ready || Boolean(busy)}
								onClick={() =>
									void run("Token", (app) =>
										app.authenticate(),
									)
								}
							>
								{busy === "Token" ? (
									<Loader2 className="animate-spin" />
								) : (
									<KeyRound />
								)}
								Get a token
							</Button>
							<Button
								variant="outline"
								disabled={!ready || Boolean(busy)}
								onClick={() =>
									void run("Payment methods", async (app) => {
										const available =
											await app.paymentMethods.list();
										setMethods(available.items);
										// Adopt the first method so the payment form is usable immediately.
										if (
											available.items[0] &&
											!form.payment_method
										) {
											setForm((current) => ({
												...current,
												payment_method:
													available.items[0]!
														.payment_method,
											}));
										}
										return available;
									})
								}
							>
								List payment methods
							</Button>
						</div>
					</TabsContent>

					<TabsContent
						value="pay"
						className="flex flex-col gap-3 pt-3"
					>
						<div className="grid gap-3 sm:grid-cols-2">
							<div className="flex flex-col gap-2">
								<Label htmlFor="tester-method">
									Payment method
								</Label>
								<Select
									value={form.payment_method}
									onValueChange={(paymentMethod) =>
										setForm({
											...form,
											payment_method:
												paymentMethod as PaymentMethod,
										})
									}
								>
									<SelectTrigger id="tester-method">
										<SelectValue placeholder="List payment methods first" />
									</SelectTrigger>
									<SelectContent>
										{[
											...new Set(
												methods.map(
													(method) =>
														method.payment_method,
												),
											),
										].map((method) => (
											<SelectItem
												key={method}
												value={method}
											>
												{method}
											</SelectItem>
										))}
									</SelectContent>
								</Select>
							</div>
							<div className="flex flex-col gap-2">
								<Label htmlFor="tester-scheme">Scheme</Label>
								<Input
									id="tester-scheme"
									value={form.scheme}
									onChange={(event) =>
										setForm({
											...form,
											scheme: event.target.value,
										})
									}
									placeholder="Optional, provider-specific"
								/>
							</div>
							<div className="flex flex-col gap-2">
								<Label htmlFor="tester-account">Account</Label>
								<Input
									id="tester-account"
									className="font-mono"
									value={form.account}
									onChange={(event) =>
										setForm({
											...form,
											account: event.target.value,
										})
									}
								/>
							</div>
							<div className="flex flex-col gap-2">
								<Label htmlFor="tester-amount">
									Amount (minor units)
								</Label>
								<Input
									id="tester-amount"
									type="number"
									min={1}
									value={form.amount}
									onChange={(event) =>
										setForm({
											...form,
											amount:
												Number(event.target.value) || 0,
										})
									}
								/>
							</div>
							<div className="flex flex-col gap-2">
								<Label htmlFor="tester-currency">
									Currency
								</Label>
								<Input
									id="tester-currency"
									maxLength={3}
									value={form.currency}
									onChange={(event) =>
										setForm({
											...form,
											currency:
												event.target.value.toUpperCase(),
										})
									}
								/>
							</div>
							<div className="flex flex-col gap-2">
								<Label htmlFor="tester-country">Country</Label>
								<Input
									id="tester-country"
									maxLength={2}
									value={form.country}
									onChange={(event) =>
										setForm({
											...form,
											country:
												event.target.value.toUpperCase(),
										})
									}
									placeholder="Optional"
								/>
							</div>
						</div>
						<div className="flex flex-wrap gap-2">
							<Button
								disabled={
									!ready ||
									Boolean(busy) ||
									!supports("collection")
								}
								onClick={() => void pay("collection")}
							>
								{busy === "Collection" ? (
									<Loader2 className="animate-spin" />
								) : (
									<Send />
								)}
								Collect
							</Button>
							<Button
								variant="outline"
								disabled={
									!ready ||
									Boolean(busy) ||
									!supports("disbursement")
								}
								onClick={() => void pay("disbursement")}
							>
								{busy === "Disbursement" ? (
									<Loader2 className="animate-spin" />
								) : (
									<Send />
								)}
								Disburse
							</Button>
						</div>
					</TabsContent>

					<TabsContent
						value="lookup"
						className="flex flex-col gap-3 pt-3"
					>
						<p className="text-muted-foreground">
							A payment created above fills these in, so its
							status can be read back through the same app
							session.
						</p>
						<div className="grid gap-3 sm:grid-cols-2">
							<div className="flex gap-2">
								<Input
									className="font-mono"
									value={lookup.id}
									onChange={(event) =>
										setLookup({
											...lookup,
											id: event.target.value,
										})
									}
									placeholder="Transaction ID"
									aria-label="Transaction ID"
								/>
								<Button
									variant="outline"
									disabled={
										!ready || Boolean(busy) || !lookup.id
									}
									onClick={() =>
										void run("Transaction", (app) =>
											app.transactions.get(lookup.id),
										)
									}
									aria-label="Look up by ID"
								>
									<Search />
								</Button>
							</div>
							<div className="flex gap-2">
								<Input
									className="font-mono"
									value={lookup.reference}
									onChange={(event) =>
										setLookup({
											...lookup,
											reference: event.target.value,
										})
									}
									placeholder="Your reference"
									aria-label="Reference"
								/>
								<Button
									variant="outline"
									disabled={
										!ready ||
										Boolean(busy) ||
										!lookup.reference
									}
									onClick={() =>
										void run("By reference", (app) =>
											app.transactions.getByReference(
												lookup.reference,
											),
										)
									}
									aria-label="Look up by reference"
								>
									<Search />
								</Button>
							</div>
						</div>
					</TabsContent>
				</Tabs>

				{error && <p className="text-destructive">{error}</p>}
				{result && (
					<div className="flex flex-col gap-2">
						<p className="font-medium">{result.label}</p>
						<pre className="bg-muted max-h-72 overflow-auto p-3 font-mono">
							{JSON.stringify(result.body, null, 2)}
						</pre>
					</div>
				)}
			</CardContent>
		</Card>
	);
}
