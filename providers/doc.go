// Package providers defines payment-provider contracts and shared utilities for
// configuring providers, issuing requests, and normalizing provider responses.
//
// Implementations register with a Momobase instance under a provider code and
// are constructed on demand by a [Factory]. The adapters shipped with Momobase,
// in providers/dummy, are built on this package and are
// registered the same way as any third-party provider.
//
// The momobase root package re-exports this package's contract, so a provider
// may be written against either import path.
package providers
