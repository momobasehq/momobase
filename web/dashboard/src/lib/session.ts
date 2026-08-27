import type { TokenSnapshot } from "@momobase/sdk";

const storageKey = "momobase.dashboard.session";

/**
 * Session persistence deliberately keeps the **refresh** token in sessionStorage and
 * the access token in memory only.
 *
 * Without this the console logs you out on every reload despite a refresh token that
 * is valid for hours. sessionStorage rather than localStorage scopes the session to
 * the tab, so closing it ends the session and a second tab starts its own.
 */
export function loadRefreshToken(): string | undefined {
	try {
		const raw = window.sessionStorage.getItem(storageKey);
		if (!raw) return undefined;
		const parsed = JSON.parse(raw) as { refreshToken?: string };
		return parsed.refreshToken || undefined;
	} catch {
		// A corrupt or unavailable store is a cold start, not an error worth surfacing.
		return undefined;
	}
}

/** persistToken records the refresh token, or clears it when the session ends. */
export function persistToken(token: TokenSnapshot | undefined) {
	try {
		if (!token?.refreshToken) {
			window.sessionStorage.removeItem(storageKey);
			return;
		}
		window.sessionStorage.setItem(
			storageKey,
			JSON.stringify({ refreshToken: token.refreshToken }),
		);
	} catch {
		// Private-browsing modes reject writes; the session still works for this page.
	}
}

/** clearSession removes any stored session, used on logout and on a dead refresh. */
export function clearSession() {
	try {
		window.sessionStorage.removeItem(storageKey);
	} catch {
		// Nothing to clean up if the store is unavailable.
	}
}
