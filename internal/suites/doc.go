// Package suites contains the definitions and registration logic for performance
// test suites. It provides a thread-safe way to register and manage test suites,
// allowing for concurrent access and modification.
//
// The `TestSuite` struct holds information about each suite, including its name
// and whether it is read-write or read-only.
package suites
