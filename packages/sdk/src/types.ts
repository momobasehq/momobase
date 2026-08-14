export type PaymentMethod = "momo"
export type ServiceType = "collection" | "disbursement"
export type TransactionStatus = "pending" | "processing" | "succeeded" | "failed" | "unknown" | "cancelled" | "expired"

export interface APIError { code: string; message: string }
export interface APIEnvelope<T = unknown> { success: boolean; data?: T; error?: APIError; message?: string }
export interface PaginatedData<T> { page: number; total: number; items: T[]; count: number }
export interface ListOptions { page?: number; perPage?: number; signal?: AbortSignal }
export interface RequestOptions { idempotencyKey?: string; signal?: AbortSignal }

export interface PartyPayload { name?: string; email?: string; phone: string }
export interface MomoPayload { phone: string; network?: "mtn" | "airtel" | "unknown" }
interface PaymentRequest {
  payment_method: "momo"; amount: number; currency: string; country: string
  reference: string; description?: string; momo: MomoPayload
}
export interface CreateCollectionRequest extends PaymentRequest { customer: PartyPayload }
export interface CreateDisbursementRequest extends PaymentRequest { recipient: PartyPayload }
export interface CreatePaymentResponse {
  transaction_id: string; reference: string; service_type: ServiceType; payment_method: PaymentMethod
  status: TransactionStatus; selected_provider: string; provider_reference: string; message: string
}
export interface Transaction {
  id: string; app_id: string; service_type: ServiceType; payment_method: "momo"; amount: number
  currency: string; country: string; reference: string; idempotency_key: string; status: TransactionStatus
  selected_route_id?: string; selected_provider_account_id?: string; provider_reference?: string
  customer_phone?: string; customer_email?: string; customer_name?: string; description?: string
  created_at: string; updated_at: string
}
export interface OAuthTokenResponse {
  access_token: string; token_type: string; expires_in: number; refresh_token?: string; scope?: string
  app_id?: string; app_name?: string; credential_id?: string; client_id?: string
}

export interface AdminUser { id: string; name: string; email: string; role: string; status: string; created_at: string; updated_at: string }
export interface App {
  id: string; name: string; description: string; status: string; environment: string
  created_by?: string; created_at: string; updated_at: string
}
export interface AppCredential {
  id: string; app_id: string; name: string; client_id: string; status: string; scopes: string
  last_used_at?: string; expires_at?: string; created_by?: string; created_at: string; updated_at: string
}
export interface CreatedCredential { credential: AppCredential; client_secret: string }
export interface ProviderAccount {
  id: string; provider_code: string; name: string; environment: string
  countries: string[]; active: boolean; config_version: number; created_at: string; updated_at: string
}
/** Provider codes registered in the running server, including custom providers. */
export interface ProviderRegistry { providers: string[] }
export interface PaymentRoute {
  id: string; service_type: ServiceType; payment_method: "momo"; provider_account_id: string
  priority: number; active: boolean; created_at: string; updated_at: string
}
export interface ProviderHealthSnapshot {
  provider_account_id: string; status: string; circuit_state: string; last_checked_at?: string
  last_success_at?: string; last_failure_at?: string; consecutive_failures: number; latency_ms: number
  collections_available: boolean; disbursements_available: boolean; balance_query_available: boolean
  last_error_code?: string; last_error_message?: string; created_at?: string; updated_at?: string
}
export interface AuditLog {
  id: string; actor_id: string; actor_type: string; action: string; entity_type: string; entity_id: string
  metadata_json: string; ip_address: string; user_agent: string; created_at: string; updated_at: string
}
export interface RuntimeProvider {
  provider_account_id: string; provider_code: string; config_version: number; active: boolean
  initialized: boolean; capabilities: unknown[]; countries: string[]; health?: ProviderHealthSnapshot
}
export interface ProviderBalance { currency: string; available: number; ledger: number }
export interface ProviderBalanceResult {
  provider_account_id: string; provider_code?: string; country: string; status: string; balance?: ProviderBalance; error?: string
}
export interface SystemInfo {
  app_name: string; app_env: string; db_type: string; addr: string; workers_enabled: boolean
  worker_names: string[]; go_version: string; server_time: string
}
export interface SystemHealth {
  ok: boolean; database: string; runtime_provider_count: number; active_provider_account_count: number
  workers_configured: string[]; server_time: string
}
export interface WorkerState { name: string; configured: boolean; state: string }
