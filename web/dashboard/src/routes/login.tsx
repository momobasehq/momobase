import { useState, type FormEvent } from "react";
import { Loader2 } from "lucide-react";

import { Logo } from "@/components/logo";
import { ThemeToggle } from "@/components/theme-toggle";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
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
import { useAuth } from "@/hooks/use-auth";

/** Login authenticates an administrator with the password grant. */
export function Login() {
	const { signIn } = useAuth();
	const [email, setEmail] = useState("");
	const [password, setPassword] = useState("");
	const [error, setError] = useState<string>();
	const [busy, setBusy] = useState(false);

	async function submit(event: FormEvent) {
		event.preventDefault();
		setBusy(true);
		setError(undefined);
		try {
			await signIn(email, password);
		} catch (cause) {
			setError(cause instanceof Error ? cause.message : "Sign-in failed");
		} finally {
			setBusy(false);
		}
	}

	return (
		<main className="relative flex min-h-svh items-center justify-center p-6">
			<div className="absolute end-4 top-4">
				<ThemeToggle />
			</div>
			<Card className="w-full max-w-sm">
				<CardHeader>
					<Logo className="size-8" />
					<CardTitle>Sign in to Momobase</CardTitle>
					<CardDescription>Administration console</CardDescription>
				</CardHeader>
				<CardContent>
					<form onSubmit={submit} className="flex flex-col gap-4">
						{error && (
							<Alert variant="destructive">
								<AlertTitle>Sign-in failed</AlertTitle>
								<AlertDescription>{error}</AlertDescription>
							</Alert>
						)}
						<div className="flex flex-col gap-2">
							<Label htmlFor="email">Email</Label>
							<Input
								id="email"
								type="email"
								autoComplete="username"
								required
								value={email}
								onChange={(event) =>
									setEmail(event.target.value)
								}
							/>
						</div>
						<div className="flex flex-col gap-2">
							<Label htmlFor="password">Password</Label>
							<Input
								id="password"
								type="password"
								autoComplete="current-password"
								required
								value={password}
								onChange={(event) =>
									setPassword(event.target.value)
								}
							/>
						</div>
						<Button type="submit" disabled={busy}>
							{busy && <Loader2 className="animate-spin" />}
							Sign in
						</Button>
					</form>
				</CardContent>
			</Card>
		</main>
	);
}
