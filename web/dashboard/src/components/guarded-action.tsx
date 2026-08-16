import type { ComponentProps, ReactNode } from "react"

import { Button } from "@/components/ui/button"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { useAuth, type Role } from "@/hooks/use-auth"

interface GuardedActionProps extends ComponentProps<typeof Button> {
  /** The role this action requires; super_admin always satisfies it. */
  role: Role
  children: ReactNode
}

/**
 * GuardedAction renders a mutating control that is **disabled** rather than hidden
 * when the signed-in admin lacks the role for it, with a tooltip explaining why.
 *
 * Hiding controls teaches operators the feature does not exist; disabling teaches
 * them who to ask. The server remains the real gate either way — this only spares a
 * predictable round trip to a 403.
 */
export function GuardedAction({ role, children, disabled, ...props }: GuardedActionProps) {
  const { can } = useAuth()
  const allowed = can(role)

  if (allowed) {
    return (
      <Button disabled={disabled} {...props}>
        {children}
      </Button>
    )
  }

  return (
    <Tooltip>
      {/* A disabled button emits no pointer events, so the trigger wraps it. */}
      <TooltipTrigger render={<span className="inline-flex" />}>
        <Button disabled {...props}>
          {children}
        </Button>
      </TooltipTrigger>
      <TooltipContent>Requires the {role.replaceAll("_", " ")} role.</TooltipContent>
    </Tooltip>
  )
}
