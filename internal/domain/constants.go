package domain

const (
	CountryGlobal = "GLOBAL"

	ServiceCollection   = "collection"
	ServiceDisbursement = "disbursement"

	PaymentMethodMomo = "momo"

	TxPending    = "pending"
	TxProcessing = "processing"
	TxSucceeded  = "succeeded"
	TxFailed     = "failed"
	TxUnknown    = "unknown"
	TxCancelled  = "cancelled"
	TxExpired    = "expired"

	ProviderHealthy       = "healthy"
	ProviderDegraded      = "degraded"
	ProviderDown          = "down"
	ProviderUnknown       = "unknown"
	ProviderDisabled      = "disabled"
	ProviderMisconfigured = "misconfigured"

	CircuitClosed   = "closed"
	CircuitOpen     = "open"
	CircuitHalfOpen = "half_open"
)
