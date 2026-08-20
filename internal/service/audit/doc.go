// Package audit records security- and administration-relevant actions.
//
// It is a leaf: it depends only on the database and the domain models, so any
// service package may take an audit recorder without creating a cycle. Writes are
// best-effort by design — a failed audit insert is logged, never returned, so it
// cannot fail the administrative action it describes.
package audit
