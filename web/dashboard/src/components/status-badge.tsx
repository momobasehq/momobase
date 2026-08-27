import { Badge } from "@/components/ui/badge";
import { titleCase } from "@/lib/format";

type Variant = "default" | "secondary" | "destructive" | "outline";

/** Maps every status string the API reports onto a badge variant. */
const variants: Record<string, Variant> = {
	succeeded: "default",
	active: "default",
	healthy: "default",
	up: "default",
	running: "default",
	processing: "secondary",
	pending: "secondary",
	unknown: "secondary",
	degraded: "secondary",
	half_open: "secondary",
	sandbox: "secondary",
	failed: "destructive",
	expired: "destructive",
	cancelled: "destructive",
	disabled: "destructive",
	suspended: "destructive",
	revoked: "destructive",
	down: "destructive",
	open: "destructive",
	inactive: "outline",
	closed: "outline",
	stopped: "outline",
};

/** StatusBadge renders an API status consistently wherever it appears. */
export function StatusBadge({ status }: { status?: string }) {
	if (!status) return <span className="text-muted-foreground">—</span>;
	return (
		<Badge variant={variants[status.toLowerCase()] ?? "outline"}>
			{titleCase(status)}
		</Badge>
	);
}
