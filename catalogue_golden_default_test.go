//go:build !legacytransport

package multiversion_test

// catalogueGoldenFile is the default-build catalogue record. The
// legacytransport build records a different file because it restores the
// Login-first catalogue entries.
const catalogueGoldenFile = "catalogue_default.json"
