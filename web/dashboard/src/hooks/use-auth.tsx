import { createContext, use, useCallback, useMemo, useRef, useState, type ReactNode } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { MomobaseAdminClient, type AdminUser } from "@momobase/sdk"

import { clearSession, loadRefreshToken, persistToken } from "@/lib/session"

/** The wildcard the super_admin role holds. It grants every permission, including
 * ones a later release adds, so the client must honour it rather than enumerate. */
const wildcard = "*"

interface AuthValue {
  client: MomobaseAdminClient
  me?: AdminUser
  /** True while the stored session is being revalidated on a cold load. */
  restoring: boolean
  signedIn: boolean
  signIn: (email: string, password: string) => Promise<void>
  signOut: () => Promise<void>
  /** Reports whether the signed-in admin holds a permission, by its code. */
  can: (permission: string) => boolean
}

const AuthContext = createContext<AuthValue | undefined>(undefined)

function createClient(onToken: (t: Parameters<typeof persistToken>[0]) => void) {
  // Same-origin: the dashboard is served by the very binary it administers.
  return new MomobaseAdminClient({ baseUrl: import.meta.env.VITE_API_URL || window.location.origin, onTokenChange: onToken })
}

/** AuthProvider owns the admin client and the signed-in identity. */
export function AuthProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient()
  const [me, setMe] = useState<AdminUser>()
  const [restoring, setRestoring] = useState(true)
  const client = useMemo(() => createClient(persistToken), [])
  const restored = useRef(false)

  // Restore once per mount. A stored refresh token is installed with no lifetime, so
  // the first request refreshes it rather than spending a round trip to learn it is
  // stale; if that fails the session is genuinely over.
  if (!restored.current) {
    restored.current = true
    const refreshToken = loadRefreshToken()
    if (!refreshToken) {
      setRestoring(false)
    } else {
      client.setAccessToken("", refreshToken)
      client.users
        .me()
        .then(setMe)
        .catch(() => {
          clearSession()
          client.clearToken()
        })
        .finally(() => setRestoring(false))
    }
  }

  const signIn = useCallback(
    async (email: string, password: string) => {
      client.setCredentials(email, password)
      setMe(await client.users.me())
    },
    [client],
  )

  const signOut = useCallback(async () => {
    try {
      await client.logout()
    } catch {
      // A server-side failure must not strand the user in a session they left.
    }
    client.clearToken()
    clearSession()
    setMe(undefined)
    queryClient.clear()
  }, [client, queryClient])

  // Gating on permissions rather than on a role name is what lets an operator create
  // a role the client has never heard of and have the UI reflect it correctly.
  const can = useCallback(
    (permission: string) => {
      const held = me?.permissions ?? []
      return held.includes(permission) || held.includes(wildcard)
    },
    [me],
  )

  const value = useMemo<AuthValue>(
    () => ({ client, me, restoring, signedIn: Boolean(me), signIn, signOut, can }),
    [client, me, restoring, signIn, signOut, can],
  )
  return <AuthContext value={value}>{children}</AuthContext>
}

/** useAuth exposes the admin client and the signed-in identity. */
export function useAuth() {
  const value = use(AuthContext)
  if (!value) throw new Error("useAuth must be used inside an AuthProvider")
  return value
}
