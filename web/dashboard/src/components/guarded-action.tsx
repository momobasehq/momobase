import type { ComponentProps, ReactNode } from "react"

import { Button } from "@/components/ui/button"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import type { PermissionCode } from "@momobase/sdk"

import { useAuth } from "@/hooks/use-auth"

interface GuardedActionProps extends ComponentProps<typeof Button> {
  /** The permission code this action requires; the wildcard always satisfies it. */
  permission: PermissionCode
  children: ReactNode
}

/**
 * GuardedAction renders a mutating control that is **disabled** rather than hidden
 * when the signed-in admin lacks the permission for it, naming it in a tooltip.
 *
 * Hiding controls teaches operators the feature does not exist; disabling teaches
 * them who to ask. The server remains the real gate either way — this only spares a
 * predictable round trip to a 403.
 */
export function GuardedAction({ permission, children, disabled, ...props }: GuardedActionProps) {
  const { can } = useAuth()
  const allowed = can(permission)

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
      <TooltipContent>Requires the <code>{permission}</code> permission.</TooltipContent>
    </Tooltip>
  )
}
