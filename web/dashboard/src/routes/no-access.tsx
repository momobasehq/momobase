import { Logo } from "@/components/logo"

/**
 * NoAccess is the fallback for a path the signed-in administrator cannot reach, and the
 * landing screen for a role that grants nothing.
 *
 * It has no nav item of its own, so it is only ever arrived at — by a deep link into a
 * screen the role does not permit, or by signing in with nothing to show. It stays
 * deliberately quiet: an operator who cannot see a screen has nothing to act on here,
 * and the tooltip on a disabled control is where "ask someone" belongs.
 */
export function NoAccess() {
  return (
    <div className="flex min-h-[60svh] flex-col items-center justify-center gap-6 p-6 text-center">
      <Logo className="text-muted-foreground/25 size-40" />
      <div className="flex flex-col gap-1">
        <p className="font-medium">Nothing to show here</p>
        <p className="text-muted-foreground max-w-sm">
          Your role does not include permission for this screen. Ask an administrator to grant it.
        </p>
      </div>
    </div>
  )
}
