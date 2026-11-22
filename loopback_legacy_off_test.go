//go:build !legacytransport

package multiversion_test

// legacyTransportEnabled reports whether this build carries the legacy
// transport fork. Without the tag, 419/475/486 cannot complete their RakNet
// v10 / Login-first handshake, so the loopback harness skips them.
func legacyTransportEnabled() bool { return false }
