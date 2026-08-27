import { useState } from "react";
import { Loader2, Play } from "lucide-react";

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
import { Textarea } from "@/components/ui/textarea";
import { StatusBadge } from "@/components/status-badge";

const methods = ["GET", "POST", "PATCH"] as const;

interface Result {
	status: number;
	durationMs: number;
	body: string;
}

/**
 * ApiTester issues arbitrary admin API calls from the browser session.
 *
 * It deliberately uses `fetch` directly rather than the SDK: the point is to see what
 * the API actually returns for a hand-written request, including the error envelopes
 * the SDK would otherwise unwrap into exceptions.
 */
export function ApiTester() {
	const [method, setMethod] = useState<(typeof methods)[number]>("GET");
	const [path, setPath] = useState("/api/admin/system/health");
	const [body, setBody] = useState("");
	const [busy, setBusy] = useState(false);
	const [result, setResult] = useState<Result>();
	const [error, setError] = useState<string>();

	async function send() {
		setBusy(true);
		setError(undefined);
		const started = performance.now();
		try {
			const headers: Record<string, string> = {};
			if (method !== "GET" && body.trim())
				headers["Content-Type"] = "application/json";
			const response = await fetch(path, {
				method,
				headers,
				body: method === "GET" || !body.trim() ? undefined : body,
				// Same-origin cookies are not used; the session token lives in memory, so the
				// tester exercises exactly what an unauthenticated caller would see.
				credentials: "omit",
			});
			const text = await response.text();
			let formatted = text;
			try {
				formatted = JSON.stringify(JSON.parse(text), null, 2);
			} catch {
				// Not JSON — show the raw body.
			}
			setResult({
				status: response.status,
				durationMs: Math.round(performance.now() - started),
				body: formatted,
			});
		} catch (cause) {
			setError(cause instanceof Error ? cause.message : "Request failed");
			setResult(undefined);
		} finally {
			setBusy(false);
		}
	}

	return (
		<Card>
			<CardHeader>
				<CardTitle>API tester</CardTitle>
				<CardDescription>
					Issue a request and see the raw response envelope.
				</CardDescription>
			</CardHeader>
			<CardContent className="flex flex-col gap-4">
				<div className="flex flex-col gap-2 sm:flex-row">
					<Select
						value={method}
						onValueChange={(value) =>
							setMethod(
								(value as (typeof methods)[number]) ?? "GET",
							)
						}
					>
						<SelectTrigger
							className="sm:w-32"
							aria-label="HTTP method"
						>
							<SelectValue />
						</SelectTrigger>
						<SelectContent>
							{methods.map((value) => (
								<SelectItem key={value} value={value}>
									{value}
								</SelectItem>
							))}
						</SelectContent>
					</Select>
					<Input
						value={path}
						onChange={(event) => setPath(event.target.value)}
						className="flex-1 font-mono"
						aria-label="Path"
					/>
					<Button
						onClick={() => void send()}
						disabled={busy || !path}
					>
						{busy ? <Loader2 className="animate-spin" /> : <Play />}
						Send
					</Button>
				</div>

				{method !== "GET" && (
					<div className="flex flex-col gap-2">
						<Label htmlFor="tester-body">Request body (JSON)</Label>
						<Textarea
							id="tester-body"
							rows={6}
							className="font-mono"
							value={body}
							onChange={(event) => setBody(event.target.value)}
						/>
					</div>
				)}

				{error && <p className="text-destructive">{error}</p>}

				{result && (
					<div className="flex flex-col gap-2">
						<div className="flex items-center gap-3">
							<StatusBadge
								status={
									result.status < 400 ? "succeeded" : "failed"
								}
							/>
							<span className="font-medium">{result.status}</span>
							<span className="text-muted-foreground">
								{result.durationMs} ms
							</span>
						</div>
						<pre className="bg-muted max-h-96 overflow-auto p-3 font-mono">
							{result.body}
						</pre>
					</div>
				)}
			</CardContent>
		</Card>
	);
}
