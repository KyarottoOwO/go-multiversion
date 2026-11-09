//go:build !legacytransport

package multiversion_test

import "testing"

// The fork deliberately advertises the Login-first families in every build, so
// the catalogue is the full 15 rather than upstream's 12 modern families: the
// server accepts 1.16.100, 1.18.2 and 1.18.12 clients without being rebuilt
// with the legacytransport build tag.
func TestForkCatalogueAdvertisesLegacyTransports(t *testing.T) {
	all := effectsProtocols(t)
	if len(all) != 15 {
		t.Fatalf("got %d adapters, want the 15 families this fork serves", len(all))
	}
	seen := map[int32]bool{}
	for _, p := range all {
		seen[p.ID()] = true
	}
	for _, id := range []int32{419, 475, 486} {
		if !seen[id] {
			t.Fatalf("legacy protocol %d missing from the catalogue", id)
		}
	}
}
