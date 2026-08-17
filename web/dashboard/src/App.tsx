import { Route, Routes } from "react-router"

import { AppShell } from "@/components/layout/app-shell"
import { Skeleton } from "@/components/ui/skeleton"
import { useAuth } from "@/hooks/use-auth"
import { ApiTester } from "@/routes/api-tester"
import { AppDetail } from "@/routes/app-detail"
import { Apps } from "@/routes/apps"
import { Audit } from "@/routes/audit"
import { Login } from "@/routes/login"
import { Operations } from "@/routes/operations"
import { Overview } from "@/routes/overview"
import { PaymentRoutes } from "@/routes/payment-routes"
import { Roles } from "@/routes/roles"
import { Providers } from "@/routes/providers"
import { Transactions } from "@/routes/transactions"
import { Users } from "@/routes/users"

/** App gates the route table on an authenticated session. */
export function App() {
  const { signedIn, restoring } = useAuth()

  // Rendering the login form while a stored session is still being revalidated would
  // flash sign-in at someone who is already signed in.
  if (restoring) {
    return (
      <div className="flex min-h-svh items-center justify-center p-6">
        <Skeleton className="h-32 w-full max-w-sm" />
      </div>
    )
  }

  if (!signedIn) return <Login />

  return (
    <Routes>
      <Route element={<AppShell />}>
        <Route index element={<Overview />} />
        <Route path="transactions" element={<Transactions />} />
        <Route path="apps" element={<Apps />} />
        <Route path="apps/:appId" element={<AppDetail />} />
        <Route path="providers" element={<Providers />} />
        <Route path="routes" element={<PaymentRoutes />} />
        <Route path="users" element={<Users />} />
        <Route path="roles" element={<Roles />} />
        <Route path="operations" element={<Operations />} />
        <Route path="audit" element={<Audit />} />
        <Route path="api-tester" element={<ApiTester />} />
        {/* An unknown hash route lands on the overview rather than a blank page. */}
        <Route path="*" element={<Overview />} />
      </Route>
    </Routes>
  )
}
