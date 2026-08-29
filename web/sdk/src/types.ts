/** Payment rails accepted by Momobase. */
export const PaymentMethods = [
	"momo",
	"card",
	"bank_transfer",
	"wallet",
] as const;
export type PaymentMethod = (typeof PaymentMethods)[number];
export type ServiceType = "collection" | "disbursement";
export type TransactionStatus =
	| "pending"
	| "processing"
	| "succeeded"
	| "failed"
	| "unknown"
	| "cancelled"
	| "expired";
export type ChargeType = "flat" | "percentage";
export interface ChargeRule {
	type: ChargeType;
	value: number;
}
export interface ChargeSchedule {
	collection: ChargeRule;
	disbursement: ChargeRule;
}

export interface APIError {
	code: string;
	message: string;
}
export interface APIEnvelope<T = unknown> {
	success: boolean;
	data?: T;
	error?: APIError;
	message?: string;
}
export interface PaginatedData<T> {
	page: number;
	total: number;
	items: T[];
	count: number;
}
export interface ListOptions {
	page?: number;
	perPage?: number;
	signal?: AbortSignal;
}
export interface RequestOptions {
	idempotencyKey?: string;
	signal?: AbortSignal;
}

export interface PartyPayload {
	name?: string;
	email?: string;
}
/** One payment method this deployment can currently serve. Fetch these before
 * collecting any payment details: the list only contains methods that will route. */
export interface AvailablePaymentMethod {
	service_type: ServiceType;
	payment_method: PaymentMethod;
}
export interface AvailablePaymentMethods {
	items: AvailablePaymentMethod[];
	count: number;
}
/** The payment payload is flat, matching the order a checkout collects it in:
 * `payment_method` and `scheme` come from the method the user picked, and `account`
 * plus `metadata` are the details they entered.
 *
 * `account` is provider-specific — a mobile number, bank account, card token, or
 * wallet address — and is validated by the provider the request routes to, not by
 * the engine. `scheme` is likewise free-form; the provider interprets it. */
interface PaymentRequest {
	payment_method: PaymentMethod;
	scheme?: string;
	account: string;
	amount: number;
	currency: string;
	country: string;
	reference: string;
	description?: string;
	metadata?: Record<string, unknown>;
}
export interface CreateCollectionRequest extends PaymentRequest {
	customer?: PartyPayload;
}
export interface CreateDisbursementRequest extends PaymentRequest {
	recipient?: PartyPayload;
}
export interface CreatePaymentResponse {
	transaction_id: string;
	reference: string;
	service_type: ServiceType;
	payment_method: PaymentMethod;
	status: TransactionStatus;
	selected_provider: string;
	provider_reference: string;
	platform_fee: number;
	message: string;
}
export interface Transaction {
	id: string;
	app_id: string;
	service_type: ServiceType;
	payment_method: PaymentMethod;
	amount: number;
	currency: string;
	country?: string;
	reference: string;
	idempotency_key: string;
	status: TransactionStatus;
	selected_route_id?: string;
	selected_provider_account_id?: string;
	provider_reference?: string;
	customer_account?: string;
	customer_email?: string;
	customer_name?: string;
	description?: string;
	platform_fee: number;
	created_at: string;
	updated_at: string;
}
export interface AdminTransaction extends Transaction {
	provider_fee: number;
	reconciliation_attempts: number;
	last_reconciled_at?: string;
	next_reconcile_at?: string;
}
export interface OAuthTokenResponse {
	access_token: string;
	token_type: string;
	expires_in: number;
	refresh_token?: string;
	scope?: string;
	app_id?: string;
	app_name?: string;
	credential_id?: string;
	client_id?: string;
}

/** A permission audience. Admin permissions are granted through roles; app
 * permissions are granted to a credential as a scope. The two are separate
 * namespaces, so a code may exist in both and mean different things. */
export type PermissionAudience = "admin" | "app";
/** One entry of the server's seeded permission catalogue. Never hardcode these —
 * fetch them, so a permission added by a later release appears without a release
 * of this client. */
export interface Permission {
	id: string;
	code: string;
	audience: PermissionAudience;
	description: string;
	created_at: string;
	updated_at: string;
}
export interface PermissionList {
	items: Permission[];
	count: number;
}
/** A named set of administrative permissions. `system` roles are seeded and
 * read-only; only custom roles can be changed or deleted. */
export interface Role {
	id: string;
	name: string;
	description: string;
	system: boolean;
	permissions: Permission[];
	created_at: string;
	updated_at: string;
}
export interface RoleList {
	items: Role[];
	count: number;
}
/** Creates or replaces a role. `name` is ignored when updating: the path carries it,
 * and a role's name is its identity because AdminUser.role refers to it. */
export interface RoleRequest {
	name?: string;
	description?: string;
	permissions: string[];
}
/** `permissions` is the role's effective set, resolved per request rather than read
 * from the token, and is present on the signed-in administrator from `me()`. Gate UI
 * on it rather than on `role`. */
export interface AdminUser {
	id: string;
	name: string;
	email: string;
	role: string;
	status: string;
	permissions?: string[];
	created_at: string;
	updated_at: string;
}
export interface App {
	id: string;
	name: string;
	description: string;
	status: string;
	environment: string;
	currency: string;
	charges: ChargeSchedule;
	created_by?: string;
	created_at: string;
	updated_at: string;
}
export interface AppCredential {
	id: string;
	app_id: string;
	name: string;
	client_id: string;
	status: string;
	scopes: string;
	last_used_at?: string;
	expires_at?: string;
	created_by?: string;
	created_at: string;
	updated_at: string;
}
export interface CreatedCredential {
	credential: AppCredential;
	client_secret: string;
}
export interface ProviderAccount {
	id: string;
	provider_code: string;
	name: string;
	environment: string;
	country: string;
	currency: string;
	charges: ChargeSchedule;
	active: boolean;
	config_version: number;
	config_hash: string;
	created_at: string;
	updated_at: string;
}
export interface ProviderCapability {
	service_type: ServiceType;
	payment_method: PaymentMethod;
}
/** Provider codes registered in the running server, including custom providers. */
export interface ProviderRegistry {
	providers: string[];
}
export interface PaymentRoute {
	id: string;
	service_type: ServiceType;
	payment_method: PaymentMethod;
	provider_account_id: string;
	provider_name: string;
	priority: number;
	active: boolean;
	created_at: string;
	updated_at: string;
}
export interface ProviderHealthSnapshot {
	provider_account_id: string;
	provider_name: string;
	status: string;
	circuit_state: string;
	last_checked_at?: string;
	last_success_at?: string;
	last_failure_at?: string;
	consecutive_failures: number;
	latency_ms: number;
	collections_available: boolean;
	disbursements_available: boolean;
	balance_query_available: boolean;
	last_error_code?: string;
	last_error_message?: string;
	created_at?: string;
	updated_at?: string;
}
export interface AuditLog {
	id: string;
	actor_id: string;
	actor_type: string;
	action: string;
	entity_type: string;
	entity_id: string;
	metadata_json: string;
	ip_address: string;
	user_agent: string;
	created_at: string;
	updated_at: string;
}
export interface RuntimeProvider {
	provider_account_id: string;
	provider_name: string;
	provider_code: string;
	config_version: number;
	active: boolean;
	initialized: boolean;
	capabilities: ProviderCapability[];
	country: string;
	currency: string;
	health?: ProviderHealthSnapshot;
}
export interface ProviderBalance {
	currency: string;
	available: number;
	ledger: number;
}
export interface ProviderBalanceResult {
	provider_account_id: string;
	provider_code?: string;
	country: string;
	status: string;
	balance?: ProviderBalance;
	error?: string;
}
export interface SystemInfo {
	app_name: string;
	app_env: string;
	db_type: string;
	addr: string;
	workers_enabled: boolean;
	worker_names: string[];
	go_version: string;
	server_time: string;
}
export interface SystemHealth {
	ok: boolean;
	database: string;
	runtime_provider_count: number;
	active_provider_account_count: number;
	workers_configured: string[];
	server_time: string;
}
export interface WorkerState {
	name: string;
	configured: boolean;
	state: string;
}

/** Transaction counts for one bucket, split by service. */
export interface ServiceCounts {
	collection: number;
	disbursement: number;
}
/** One point on an analytics time series. Quiet periods are present and zeroed, so a
 * chart shows a gap in traffic rather than joining a line across it. */
export interface AnalyticsBucket {
	period: string;
	total: number;
	by_service: ServiceCounts;
	succeeded: number;
	failed: number;
}
/** Volume for one currency. Amounts are in that currency's minor unit and are
 * deliberately never summed across currencies — the total would mean nothing. */
export interface CurrencyVolume {
	currency: string;
	count: number;
	amount: number;
}
export interface TransactionAnalytics {
	from: string;
	to: string;
	interval: "day" | "hour";
	buckets: AnalyticsBucket[];
	total: number;
	by_service: ServiceCounts;
	volume: CurrencyVolume[];
}
/** Narrows an analytics query. Every field is optional; the server defaults to the
 * last 30 days bucketed by day. */
export interface AnalyticsQuery {
	from?: string;
	to?: string;
	interval?: "day" | "hour";
	appId?: string;
	providerAccountId?: string;
}
