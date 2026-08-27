import {
	Activity,
	AppWindow,
	FileClock,
	LayoutDashboard,
	Plug,
	Receipt,
	Route as RouteIcon,
	ShieldCheck,
	TerminalSquare,
	Users,
} from "lucide-react";
import { AdminPermissions, type PermissionCode } from "@momobase/sdk";

/** NavItem is one sidebar entry and the permission required to reach it. */
export interface NavItem {
	/** Path relative to the router's root. */
	to: string;
	label: string;
	icon: typeof Activity;
	/** The permission the screen's own data needs. Codes come from the SDK so a typo is
	 * a compile error rather than a permission that silently never matches. */
	permission: PermissionCode;
	/** Matches the path exactly, for the index route. */
	end?: boolean;
}

/** NavGroup is a labelled section of the sidebar. */
export interface NavGroup {
	label: string;
	items: NavItem[];
}

/**
 * navigation is the single description of the console's screens, their permissions, and
 * their order. The sidebar and the route table both read it, so a screen cannot appear
 * in one and be missing from the other.
 *
 * A screen's permission is the one its *primary* data needs. Screens that also show a
 * second resource gate that section separately — Providers pairs accounts with
 * balances, which the read_only role splits.
 */
export const navigation: NavGroup[] = [
	{
		label: "Overview",
		items: [
			{
				to: "/",
				label: "Dashboard",
				icon: LayoutDashboard,
				permission: AdminPermissions.systemRead,
				end: true,
			},
			{
				to: "/transactions",
				label: "Transactions",
				icon: Receipt,
				permission: AdminPermissions.transactionsRead,
			},
		],
	},
	{
		label: "Configuration",
		items: [
			{
				to: "/apps",
				label: "Apps",
				icon: AppWindow,
				permission: AdminPermissions.appsRead,
			},
			{
				to: "/providers",
				label: "Providers",
				icon: Plug,
				permission: AdminPermissions.providersRead,
			},
			{
				to: "/routes",
				label: "Routes",
				icon: RouteIcon,
				permission: AdminPermissions.routesRead,
			},
		],
	},
	{
		label: "Administration",
		items: [
			{
				to: "/users",
				label: "Users",
				icon: Users,
				permission: AdminPermissions.usersRead,
			},
			{
				to: "/roles",
				label: "Roles",
				icon: ShieldCheck,
				permission: AdminPermissions.rolesRead,
			},
			{
				to: "/operations",
				label: "Operations",
				icon: Activity,
				permission: AdminPermissions.systemRead,
			},
			{
				to: "/audit",
				label: "Audit log",
				icon: FileClock,
				permission: AdminPermissions.auditRead,
			},
			// The tester issues arbitrary requests with the caller's own session, so it can
			// never exceed what they already hold. Reading the system is the lowest bar that
			// still keeps it out of a role with no read access at all.
			{
				to: "/api-tester",
				label: "API tester",
				icon: TerminalSquare,
				permission: AdminPermissions.systemRead,
			},
		],
	},
];

/** allNavItems is every item across every group, for route construction. */
export const allNavItems: NavItem[] = navigation.flatMap(
	(group) => group.items,
);

/** Titles keyed by the first path segment, for the page header. */
export const titles: Record<string, string> = Object.fromEntries(
	allNavItems.map((item) => [item.to.replace(/^\//, ""), item.label]),
);
