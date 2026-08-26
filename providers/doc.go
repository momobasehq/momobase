// Package providers defines payment-provider contracts and shared utilities for
// configuring providers, issuing requests, and normalizing provider responses.
//
// Implementations register with a Momobase instance under a provider code and
// are constructed on demand by a [Factory]. The optional adapters in
// providers/dummy and providers/mtn are built on this package and must be
// registered explicitly by the application that uses them.
//
// The momobase root package re-exports this package's contract, so a provider
// may be written against either import path.
package providers
