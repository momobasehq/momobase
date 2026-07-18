package providers

import (
	"fmt"
	"log/slog"
)

type Factory func(*slog.Logger) PaymentProvider
type Registry map[string]Factory

func NewRegistry() Registry {
	return Registry{}
}

func (r Registry) Register(code string, f Factory) {
	r[code] = f
}

func (r Registry) Create(code string, log *slog.Logger) (PaymentProvider, error) {
	if f := r[code]; f != nil {
		return f(log), nil
	}
	return nil, fmt.Errorf("provider factory not registered: %s", code)
}

func Supports(caps []Capability, service, method string) bool {
	for _, c := range caps {
		if c.ServiceType == service && c.PaymentMethod == method {
			return true
		}
	}
	return false
}
