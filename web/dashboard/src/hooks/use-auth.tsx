import {
	createContext,
	use,
	useCallback,
	useMemo,
	useRef,
	useState,
	type ReactNode,
} from "react";
import { useQueryClient } from "@tanstack/react-query";
import {
	MomobaseAdminClient,
	permitted,
	type AdminUser,
	type PermissionCode,
} from "@momobase/sdk";

import { clearSession, loadRefreshToken, persistToken } from "@/lib/session";

interface AuthValue {
	client: MomobaseAdminClient;
	me?: AdminUser;
	/** True while the stored session is being revalidated on a cold load. */
	restoring: boolean;
	signedIn: boolean;
	signIn: (email: string, password: string) => Promise<void>;
	signOut: () => Promise<void>;
	/** Reports whether the signed-in admin holds a permission, by its code. */
	can: (permission: PermissionCode) => boolean;
}

const AuthContext = createContext<AuthValue | undefined>(undefined);

function createClient(
	onToken: (t: Parameters<typeof persistToken>[0]) => void,
) {
	// Same origin by default, since the dashboard is normally served by the very binary it
	// administers. VITE_API_URL points it elsewhere when the two are deployed separately.
	return new MomobaseAdminClient({
		baseUrl: import.meta.env.VITE_API_URL || window.location.origin,
		onTokenChange: onToken,
	});
}

/** AuthProvider owns the admin client and the signed-in identity. */
export function AuthProvider({ children }: { children: ReactNode }) {
	const queryClient = useQueryClient();
	const [me, setMe] = useState<AdminUser>();
	const [restoring, setRestoring] = useState(true);
	const client = useMemo(() => createClient(persistToken), []);
	const restored = useRef(false);

	// Restore once per mount. A stored refresh token is installed with no lifetime, so
	// the first request refreshes it rather than spending a round trip to learn it is
	// stale; if that fails the session is genuinely over.
	if (!restored.current) {
		restored.current = true;
		const refreshToken = loadRefreshToken();
		if (!refreshToken) {
			setRestoring(false);
		} else {
			client.setAccessToken("", refreshToken);
			client.users
				.me()
				.then(setMe)
				.catch(() => {
					clearSession();
					client.clearToken();
				})
				.finally(() => setRestoring(false));
		}
	}

	const signIn = useCallback(
		async (email: string, password: string) => {
			client.setCredentials(email, password);
			setMe(await client.users.me());
		},
		[client],
	);

	const signOut = useCallback(async () => {
		try {
			await client.logout();
		} catch {
			// A server-side failure must not strand the user in a session they left.
		}
		client.clearToken();
		clearSession();
		setMe(undefined);
		queryClient.clear();
	}, [client, queryClient]);

	// Gating on permissions rather than on a role name is what lets an operator create
	// a role the client has never heard of and have the UI reflect it correctly. The
	// wildcard check lives in the SDK so client gating and server enforcement agree.
	const can = useCallback(
		(permission: PermissionCode) => permitted(me?.permissions, permission),
		[me],
	);

	const value = useMemo<AuthValue>(
		() => ({
			client,
			me,
			restoring,
			signedIn: Boolean(me),
			signIn,
			signOut,
			can,
		}),
		[client, me, restoring, signIn, signOut, can],
	);
	return <AuthContext value={value}>{children}</AuthContext>;
}

/** useAuth exposes the admin client and the signed-in identity. */
export function useAuth() {
	const value = use(AuthContext);
	if (!value) throw new Error("useAuth must be used inside an AuthProvider");
	return value;
}
