package providers

import (
	"fmt"
	"log/slog"
	"maps"
	"slices"
)

// Factory constructs a payment provider using the supplied logger.
type Factory func(*slog.Logger) PaymentProvider

// Registry maps provider codes to their factories.
type Registry map[string]Factory

// NewRegistry returns an empty provider registry.
func NewRegistry() Registry {
	return Registry{}
}

// Register associates code with a provider factory, replacing any factory
// previously registered under the same code.
func (r Registry) Register(code string, f Factory) {
	r[code] = f
}

// Has reports whether a factory is registered for code.
func (r Registry) Has(code string) bool {
	_, ok := r[code]
	return ok
}

// List returns the registered provider codes in ascending order.
func (r Registry) List() []string {
	return slices.Sorted(maps.Keys(r))
}

// Create constructs the provider registered for code. A nil logger is replaced
// with a discarding logger so that factories may use it unconditionally.
func (r Registry) Create(code string, log *slog.Logger) (PaymentProvider, error) {
	f := r[code]
	if f == nil {
		return nil, fmt.Errorf("provider factory not registered: %s", code)
	}
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	provider := f(log)
	if provider == nil {
		return nil, fmt.Errorf("provider factory returned no provider: %s", code)
	}
	return provider, nil
}

// Supports reports whether caps contains the requested service.
func Supports(caps []Capability, service string) bool {
	for _, c := range caps {
		if c.ServiceType == service {
			return true
		}
	}
	return false
}
