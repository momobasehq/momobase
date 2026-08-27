import type { ComponentProps } from "react";

import { cn } from "@/lib/utils";

/**
 * Logo is the Momobase mark: an M drawn as one continuous stroke whose descenders
 * merge into a solid bar — separate rails converging on one base.
 *
 * It paints in `currentColor` rather than a fixed black, so it inherits the ink of
 * whatever surface it sits on and needs no dark-mode variant.
 */
export function Logo({ className, ...props }: ComponentProps<"svg">) {
	return (
		<svg
			viewBox="0 0 32 32"
			fill="none"
			stroke="currentColor"
			strokeWidth={4}
			strokeLinecap="round"
			strokeLinejoin="round"
			role="img"
			aria-label="Momobase"
			className={cn("size-6", className)}
			{...props}
		>
			<path d="M6 21 L11 7 L16 15 L21 7 L26 21" />
			<path d="M6 25 H26" />
		</svg>
	);
}
