import { MomobaseAPIError } from "./errors.js"
import type {
  AdminUser, APIEnvelope, App, AppCredential, AuditLog, CreateCollectionRequest,
  CreateDisbursementRequest, CreatePaymentResponse, CreatedCredential, ListOptions,
  OAuthTokenResponse, PaginatedData, PaymentRoute, ProviderAccount, ProviderBalance,
  ProviderBalanceResult, ProviderHealthSnapshot, RequestOptions, RuntimeProvider,
  SystemHealth, SystemInfo, Transaction, WorkerState
} from "./types.js"

export interface MomobaseClientOptions { baseUrl: string; clientId: string; clientSecret: string; tokenSkewSeconds?: number }
export interface AdminClientOptions {
  baseUrl: string; email?: string; password?: string; accessToken?: string
  refreshToken?: string; tokenSkewSeconds?: number
}
type Method = "GET" | "POST" | "PATCH"
type CachedToken = { accessToken: string; refreshToken?: string; expiresAt: number }

const query = (o?: ListOptions) => {
  const q = new URLSearchParams()
  if (o?.page) q.set("page", String(o.page))
  if (o?.perPage) q.set("per_page", String(o.perPage))
  return q.size ? `?${q}` : ""
}
const endpoint = (path: string, id: string) => `${path}/${encodeURIComponent(id)}`
async function unwrap<T>(r: Response): Promise<T> {
  if (!r.ok) throw await MomobaseAPIError.fromResponse(r)
  const body = await r.json() as APIEnvelope<T>
  if (body && typeof body === "object" && "success" in body) {
    if (!body.success) throw new MomobaseAPIError(r.status, body.error?.code ?? "API_ERROR", body.error?.message ?? body.message ?? "API error", body)
    return body.data as T
  }
  return body as T
}
function cached(t: OAuthTokenResponse, skew: number): CachedToken {
  return { accessToken: t.access_token, refreshToken: t.refresh_token, expiresAt: Date.now() + Math.max(t.expires_in - skew, 1) * 1000 }
}
function validatePayment(kind: "collection" | "disbursement", p: CreateCollectionRequest | CreateDisbursementRequest) {
  if (p.payment_method !== "momo" || !p.momo?.phone) throw new Error("payment_method=momo and momo.phone are required")
  if (p.country.length !== 2) throw new Error("country must be a 2-letter ISO code")
  if (kind === "collection" ? !(p as CreateCollectionRequest).customer?.phone : !(p as CreateDisbursementRequest).recipient?.phone) throw new Error(`${kind} party phone is required`)
}

abstract class SessionClient {
  protected readonly baseUrl: string
  protected readonly skew: number
  protected token?: CachedToken
  constructor(baseUrl: string, skew = 30) { this.baseUrl = baseUrl.replace(/\/$/, ""); this.skew = skew }
  protected abstract authenticate(signal?: AbortSignal): Promise<OAuthTokenResponse>
  protected abstract refresh(signal?: AbortSignal): Promise<OAuthTokenResponse>
  clearToken() { this.token = undefined }
  protected setToken(t: OAuthTokenResponse) { this.token = cached(t, this.skew); return t }
  protected async bearer(signal?: AbortSignal) {
    if (!this.token) await this.authenticate(signal)
    else if (this.token.expiresAt <= Date.now()) await this.refresh(signal)
    return this.token!.accessToken
  }
  protected async request<T>(method: Method, path: string, payload?: unknown, options: RequestOptions = {}) {
    const headers: Record<string, string> = { Authorization: `Bearer ${await this.bearer(options.signal)}` }
    if (method !== "GET") headers["Content-Type"] = "application/json"
    if (options.idempotencyKey) headers["Idempotency-Key"] = options.idempotencyKey
    return unwrap<T>(await fetch(this.baseUrl + path, { method, headers, body: method === "GET" ? undefined : JSON.stringify(payload ?? {}), signal: options.signal }))
  }
  protected get<T>(path: string, options?: RequestOptions) { return this.request<T>("GET", path, undefined, options) }
  protected post<T>(path: string, payload?: unknown, options?: RequestOptions) { return this.request<T>("POST", path, payload, options) }
  protected patch<T>(path: string, payload?: unknown, options?: RequestOptions) { return this.request<T>("PATCH", path, payload, options) }
  protected async form(path: string, values: Record<string, string>, signal?: AbortSignal) {
    const r = await fetch(this.baseUrl + path, { method: "POST", headers: { "Content-Type": "application/x-www-form-urlencoded" }, body: new URLSearchParams(values), signal })
    if (!r.ok) throw await MomobaseAPIError.fromResponse(r)
    return this.setToken(await r.json() as OAuthTokenResponse)
  }
}

export class MomobaseClient extends SessionClient {
  constructor(private readonly options: MomobaseClientOptions) { super(options.baseUrl, options.tokenSkewSeconds) }
  authenticate(signal?: AbortSignal) { return this.form("/api/v1/token", { grant_type: "client_credentials", client_id: this.options.clientId, client_secret: this.options.clientSecret }, signal) }
  async refresh(signal?: AbortSignal) {
    if (!this.token?.refreshToken) return this.authenticate(signal)
    try { return await this.form("/api/v1/token/refresh", { grant_type: "refresh_token", refresh_token: this.token.refreshToken }, signal) }
    catch { this.clearToken(); return this.authenticate(signal) }
  }
  readonly collections = { create: (p: CreateCollectionRequest, o: RequestOptions = {}) => { validatePayment("collection", p); return this.post<CreatePaymentResponse>("/api/v1/collections", p, o) } }
  readonly disbursements = { create: (p: CreateDisbursementRequest, o: RequestOptions = {}) => { validatePayment("disbursement", p); return this.post<CreatePaymentResponse>("/api/v1/disbursements", p, o) } }
  readonly transactions = {
    get: (id: string, o: RequestOptions = {}) => this.get<Transaction>(endpoint("/api/v1/transactions", id), o),
    getByReference: (ref: string, o: RequestOptions = {}) => this.get<Transaction>(endpoint("/api/v1/transactions/by-reference", ref), o)
  }
}

export class MomobaseAdminClient extends SessionClient {
  private email?: string
  private password?: string
  constructor(o: AdminClientOptions) {
    super(o.baseUrl, o.tokenSkewSeconds); this.email = o.email; this.password = o.password
    if (o.accessToken) this.token = { accessToken: o.accessToken, refreshToken: o.refreshToken, expiresAt: Number.MAX_SAFE_INTEGER }
  }
  setCredentials(email: string, password: string) { this.email = email; this.password = password; this.clearToken() }
  setAccessToken(accessToken: string, refreshToken?: string) { this.token = { accessToken, refreshToken, expiresAt: Number.MAX_SAFE_INTEGER } }
  authenticate(signal?: AbortSignal) {
    if (!this.email || !this.password) return Promise.reject(new Error("Admin email and password are required"))
    return this.form("/api/admin/token", { grant_type: "password", username: this.email, password: this.password }, signal)
  }
  async refresh(signal?: AbortSignal) {
    if (!this.token?.refreshToken) return this.authenticate(signal)
    try { return await this.form("/api/admin/token/refresh", { grant_type: "refresh_token", refresh_token: this.token.refreshToken }, signal) }
    catch { this.clearToken(); return this.authenticate(signal) }
  }
  logout() { return this.post<unknown>("/api/admin/logout") }
  readonly system = {
    info: () => this.get<SystemInfo>("/api/admin/system/info"), health: () => this.get<SystemHealth>("/api/admin/system/health"),
    workers: (o?: ListOptions) => this.get<PaginatedData<WorkerState>>(`/api/admin/workers${query(o)}`),
    runtimeProviders: (o?: ListOptions) => this.get<PaginatedData<RuntimeProvider>>(`/api/admin/runtime/providers${query(o)}`)
  }
  readonly users = {
    me: () => this.get<AdminUser>("/api/admin/me"), list: (o?: ListOptions) => this.get<PaginatedData<AdminUser>>(`/api/admin/users${query(o)}`),
    create: (p: { name: string; email: string; password: string; role?: string }) => this.post<AdminUser>("/api/admin/users", p),
    changePassword: (id: string, password: string) => this.patch<unknown>(endpoint("/api/admin/users", id) + "/password", { password }),
    changeStatus: (id: string, status: "active" | "inactive") => this.patch<unknown>(endpoint("/api/admin/users", id) + "/status", { status })
  }
  readonly apps = {
    list: (o?: ListOptions) => this.get<PaginatedData<App>>(`/api/admin/apps${query(o)}`), create: (p: { name: string; description?: string; environment?: "sandbox" | "production" }) => this.post<App>("/api/admin/apps", p),
    get: (id: string) => this.get<App>(endpoint("/api/admin/apps", id)), update: (id: string, p: Partial<Pick<App, "name" | "description" | "environment">>) => this.patch<App>(endpoint("/api/admin/apps", id), p),
    changeStatus: (id: string, status: "active" | "disabled" | "suspended") => this.patch<unknown>(endpoint("/api/admin/apps", id) + "/status", { status }),
    credentials: (id: string, o?: ListOptions) => this.get<PaginatedData<AppCredential>>(`${endpoint("/api/admin/apps", id)}/credentials${query(o)}`),
    createCredential: (id: string, p: { name?: string; scopes?: string; expires_at?: string }) => this.post<CreatedCredential>(endpoint("/api/admin/apps", id) + "/credentials", p),
    revokeCredential: (id: string, cid: string) => this.patch<unknown>(`${endpoint("/api/admin/apps", id)}/credentials/${encodeURIComponent(cid)}/revoke`),
    rotateCredential: (id: string, cid: string) => this.post<CreatedCredential>(`${endpoint("/api/admin/apps", id)}/credentials/${encodeURIComponent(cid)}/rotate`)
  }
  readonly providers = {
    list: (o?: ListOptions) => this.get<PaginatedData<ProviderAccount>>(`/api/admin/providers${query(o)}`),
    createAccount: (p: { provider_code: string; name: string; environment: "sandbox" | "production"; countries: string[]; config: Record<string, unknown> }) => this.post<ProviderAccount>("/api/admin/providers/accounts", p),
    updateCountries: (id: string, countries: string[]) => this.patch<unknown>(endpoint("/api/admin/providers/accounts", id) + "/countries", { countries }),
    updateConfig: (id: string, config: Record<string, unknown>) => this.patch<unknown>(endpoint("/api/admin/providers/accounts", id) + "/config", { config }),
    activate: (id: string) => this.patch<unknown>(endpoint("/api/admin/providers/accounts", id) + "/activate"), deactivate: (id: string) => this.patch<unknown>(endpoint("/api/admin/providers/accounts", id) + "/deactivate"),
    test: (id: string) => this.post<unknown>(endpoint("/api/admin/providers/accounts", id) + "/test"), balance: (id: string, country?: string) => this.get<ProviderBalance>(endpoint("/api/admin/providers/accounts", id) + "/balance" + (country ? `?country=${encodeURIComponent(country)}` : "")),
    activeBalances: (o?: ListOptions) => this.get<PaginatedData<ProviderBalanceResult>>(`/api/admin/balances/providers${query(o)}`), health: (o?: ListOptions) => this.get<PaginatedData<ProviderHealthSnapshot>>(`/api/admin/health/providers${query(o)}`)
  }
  readonly routes = {
    list: (o?: ListOptions) => this.get<PaginatedData<PaymentRoute>>(`/api/admin/routes${query(o)}`), create: (p: Omit<PaymentRoute, "id" | "created_at" | "updated_at">) => this.post<PaymentRoute>("/api/admin/routes", p),
    update: (id: string, p: { priority: number; active: boolean }) => this.patch<unknown>(endpoint("/api/admin/routes", id), p)
  }
  readonly transactions = {
    list: (o?: ListOptions) => this.get<PaginatedData<Transaction>>(`/api/admin/transactions${query(o)}`), auditLogs: (o?: ListOptions) => this.get<PaginatedData<AuditLog>>(`/api/admin/audit-logs${query(o)}`)
  }
}
