// Package testsupport builds a fully wired service stack backed by a throwaway
// in-memory database, so a test can exercise a real payment path instead of a mock.
//
// It is consumed only from external test packages (package foo_test). Importing it
// from an in-package test would be a cycle, because testsupport imports the very
// packages under test — a test that needs an unexported symbol must therefore build
// its own fixtures or stay free of the database.
package testsupport
