package domain

const (
	// ServiceCollection identifies an incoming payment collection.
	ServiceCollection = "collection"
	// ServiceDisbursement identifies an outgoing payment disbursement.
	ServiceDisbursement = "disbursement"

	// TxPending indicates that a transaction is waiting to be processed.
	TxPending = "pending"
	// TxProcessing indicates that transaction processing has started.
	TxProcessing = "processing"
	// TxSucceeded indicates that a transaction completed successfully.
	TxSucceeded = "succeeded"
	// TxFailed indicates that a transaction failed permanently.
	TxFailed = "failed"
	// TxUnknown indicates that the provider outcome is not yet known.
	TxUnknown = "unknown"
	// TxCancelled indicates that a transaction was cancelled.
	TxCancelled = "cancelled"
	// TxExpired indicates that a transaction expired before completion.
	TxExpired = "expired"

	// ProviderHealthy indicates that a provider is operating normally.
	ProviderHealthy = "healthy"
	// ProviderDegraded indicates that a provider is operating with failures.
	ProviderDegraded = "degraded"
	// ProviderDown indicates that a provider is unavailable.
	ProviderDown = "down"
	// ProviderUnknown indicates that provider health has not been established.
	ProviderUnknown = "unknown"
	// ProviderDisabled indicates that a provider is administratively disabled.
	ProviderDisabled = "disabled"
	// ProviderMisconfigured indicates that provider configuration is invalid.
	ProviderMisconfigured = "misconfigured"

	// CircuitClosed allows provider requests to proceed normally.
	CircuitClosed = "closed"
	// CircuitOpen prevents provider requests after repeated failures.
	CircuitOpen = "open"
	// CircuitHalfOpen allows a trial request after an open-circuit timeout.
	CircuitHalfOpen = "half_open"
)
