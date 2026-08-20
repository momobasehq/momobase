package bootstrap

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/momobasehq/momobase/providers"
)

// Option customizes the application constructed by NewApp.
type Option func(*options)

type options struct {
	logger   *slog.Logger
	registry providers.Registry
	extra    map[string]providers.Factory
	codes    []string
}

// WithLogger uses log instead of a logger derived from the configured log level.
func WithLogger(log *slog.Logger) Option {
	return func(o *options) { o.logger = log }
}

// withRegistry uses registry as the base set of payment providers. Providers
// supplied with WithProvider are added to it.
func withRegistry(registry providers.Registry) Option {
	return func(o *options) { o.registry = registry }
}

// WithProvider registers a provider factory under code, replacing any factory
// previously supplied for the same code.
func WithProvider(code string, factory providers.Factory) Option {
	return func(o *options) {
		if o.extra == nil {
			o.extra = map[string]providers.Factory{}
		}
		if _, exists := o.extra[code]; !exists {
			o.codes = append(o.codes, code)
		}
		o.extra[code] = factory
	}
}

func newOptions(opts []Option) *options {
	o := &options{}
	for _, opt := range opts {
		if opt != nil {
			opt(o)
		}
	}
	return o
}

// buildRegistry assembles the registry used by the application. Momobase ships
// no providers of its own, so a build that registers none is rejected rather
// than started unable to execute any payment.
func (o *options) buildRegistry() (providers.Registry, error) {
	registry := o.registry
	if registry == nil {
		registry = providers.NewRegistry()
	}
	for _, code := range o.codes {
		if strings.TrimSpace(code) == "" {
			return nil, errors.New("provider code is required")
		}
		factory := o.extra[code]
		if factory == nil {
			return nil, fmt.Errorf("provider factory is required: %s", code)
		}
		registry.Register(code, factory)
	}
	if len(registry) == 0 {
		return nil, errors.New("no payment providers registered: supply at least one with WithProvider")
	}
	return registry, nil
}
