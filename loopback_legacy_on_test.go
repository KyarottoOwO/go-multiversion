//go:build legacytransport

package multiversion_test

// legacyTransportEnabled reports whether this build carries the legacy
// transport fork, which 419/475/486 need to complete their handshake.
func legacyTransportEnabled() bool { return true }
