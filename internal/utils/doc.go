// Package utils holds small, dependency-free helpers shared across Momobase's
// service packages.
//
// Everything here is a pure function over plain values: it neither touches the
// database nor depends on any other internal package. Behaviour that belongs to a
// persisted model lives on that model in internal/domain instead.
package utils
