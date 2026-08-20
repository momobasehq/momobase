// Package utils holds small, dependency-free helpers shared across Momobase's
// packages, including the provider adapters.
//
// Everything here is a pure function over plain values: it neither touches the
// database nor depends on any other internal package. Behaviour that belongs to a
// persisted model lives on that model in internal/domain instead. The helpers an
// out-of-tree adapter needs are re-exported from the root momobase package, which
// is internal/utils' only route out of the module.
package utils
