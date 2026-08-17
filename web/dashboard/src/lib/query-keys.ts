import type { ListOptions } from "@momobase/sdk"

/** Page is the pagination slice a list query is showing. */
export type Page = Required<Pick<ListOptions, "page" | "perPage">>

/**
 * keys is the single source of query keys for the whole dashboard.
 *
 * Every key starts with its resource name so a mutation can invalidate by prefix —
 * `invalidateQueries({ queryKey: keys.apps.all })` catches every page of every app
 * list without the caller knowing which pages happen to be cached.
 */
export const keys = {
  me: ["me"] as const,
  system: {
    all: ["system"] as const,
    info: () => [...keys.system.all, "info"] as const,
    health: () => [...keys.system.all, "health"] as const,
    workers: (page: Page) => [...keys.system.all, "workers", page] as const,
    runtime: (page: Page) => [...keys.system.all, "runtime", page] as const,
  },
  authz: {
    all: ["authz"] as const,
    permissions: (audience?: string) => [...keys.authz.all, "permissions", audience ?? "all"] as const,
    roles: () => [...keys.authz.all, "roles"] as const,
  },
  users: {
    all: ["users"] as const,
    list: (page: Page) => [...keys.users.all, "list", page] as const,
  },
  apps: {
    all: ["apps"] as const,
    list: (page: Page) => [...keys.apps.all, "list", page] as const,
    detail: (id: string) => [...keys.apps.all, "detail", id] as const,
    credentials: (id: string, page: Page) => [...keys.apps.all, "credentials", id, page] as const,
  },
  providers: {
    all: ["providers"] as const,
    list: (page: Page) => [...keys.providers.all, "list", page] as const,
    registry: () => [...keys.providers.all, "registry"] as const,
    health: (page: Page) => [...keys.providers.all, "health", page] as const,
    balances: (page: Page) => [...keys.providers.all, "balances", page] as const,
  },
  routes: {
    all: ["routes"] as const,
    list: (page: Page) => [...keys.routes.all, "list", page] as const,
  },
  transactions: {
    all: ["transactions"] as const,
    list: (page: Page) => [...keys.transactions.all, "list", page] as const,
  },
  audit: {
    all: ["audit"] as const,
    list: (page: Page) => [...keys.audit.all, "list", page] as const,
  },
} as const
