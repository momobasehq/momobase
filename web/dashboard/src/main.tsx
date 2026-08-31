import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import {
	MutationCache,
	QueryCache,
	QueryClient,
	QueryClientProvider,
} from "@tanstack/react-query";
import { HashRouter } from "react-router";
import { ThemeProvider } from "next-themes";
import { MomobaseAPIError } from "momobase";
import { toast } from "sonner";

import { App } from "@/App";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { AuthProvider } from "@/hooks/use-auth";
import { clearSession } from "@/lib/session";
import "./index.css";

/**
 * One policy for every failed request, so no screen has to restate it.
 *
 * - The SDK refreshes and retries once when a request returns 401. A 401 that reaches
 *   this handler means the refreshed request was also rejected, so the session is over.
 * - 403 is a role the admin does not have. Toast it, but do **not** sign them out:
 *   losing a whole session because one control was out of reach is worse than the
 *   denial itself.
 * - 429 is the admin rate limiter, which is per remote address — behind a proxy every
 *   dashboard user shares one bucket, so the hint matters.
 */
function reportError(error: unknown) {
	if (!(error instanceof MomobaseAPIError)) {
		toast.error(error instanceof Error ? error.message : "Request failed");
		return;
	}
	switch (error.status) {
		case 401:
			clearSession();
			toast.error("Your session expired. Sign in again.");
			// A full reload is the honest reset: it drops in-memory tokens and every cache.
			window.location.reload();
			return;
		case 403:
			toast.error(`Not permitted: ${error.message}`);
			return;
		case 429:
			toast.error("Rate limited. Wait a moment before retrying.");
			return;
		default:
			toast.error(error.message);
	}
}

const queryClient = new QueryClient({
	queryCache: new QueryCache({ onError: reportError }),
	// Mutations report their own successes; this catches the ones a screen does not.
	mutationCache: new MutationCache({ onError: reportError }),
	defaultOptions: {
		queries: {
			// Refetching on focus would spend the shared rate-limit bucket on a console
			// someone leaves open in a background tab.
			refetchOnWindowFocus: false,
			staleTime: 30_000,
			retry: (failureCount, error) => {
				if (error instanceof MomobaseAPIError && error.status < 500)
					return false;
				return failureCount < 2;
			},
		},
	},
});

const root = document.getElementById("root");
if (!root) {
	throw new Error("index.html is missing the #root mount point");
}

createRoot(root).render(
	<StrictMode>
		{/* The theme applies as a class on <html>, which is what the dark variant in
        index.css keys off. "system" follows the operating system until an operator
        chooses otherwise, and disableTransitionOnChange stops every themed surface
        from animating at once when they do. */}
		<ThemeProvider
			attribute="class"
			defaultTheme="system"
			enableSystem
			disableTransitionOnChange
		>
			<QueryClientProvider client={queryClient}>
				{/* Hash routing: every route lives after the #, which browsers never send to
            the server. A hard refresh of a deep link is therefore just a request for
            the dashboard root, so no SPA fallback is needed and none can be got wrong. */}
				<HashRouter>
					<AuthProvider>
						<TooltipProvider>
							<App />
							<Toaster />
						</TooltipProvider>
					</AuthProvider>
				</HashRouter>
			</QueryClientProvider>
		</ThemeProvider>
	</StrictMode>,
);
