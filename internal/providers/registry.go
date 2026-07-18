package providers

import (
	"fmt"
	"log/slog"
)

// Factory constructs a payment provider using the supplied logger.
type Factory func(*slog.Logger) PaymentProvider

// Registry maps provider codes to their factories.
type Registry map[string]Factory

// NewRegistry returns an empty provider registry.
func NewRegistry() Registry {
	return Registry{}
}

// Register associates code with a provider factory.
func (r Registry) Register(code string, f Factory) {
	r[code] = f
}

// Create constructs the provider registered for code.
func (r Registry) Create(code string, log *slog.Logger) (PaymentProvider, error) {
	if f := r[code]; f != nil {
		return f(log), nil
	}
	return nil, fmt.Errorf("provider factory not registered: %s", code)
}

// Supports reports whether caps contains the requested service and payment method.
func Supports(caps []Capability, service, method string) bool {
	for _, c := range caps {
		if c.ServiceType == service && c.PaymentMethod == method {
			return true
		}
	}
	return false
}
