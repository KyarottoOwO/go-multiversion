package mapping_test

import (
	"bytes"
	"testing"

	v419 "github.com/shawtymarco/go-multiversion/data/v419"
	"github.com/shawtymarco/go-multiversion/mapping"
	"github.com/sandertv/gophertunnel/minecraft/nbt"
)

// biome419ValidIDs builds the set of biome IDs the 1.16.100 client can resolve
// from the mapping's name table.
func biome419ValidIDs(t *testing.T) map[uint32]struct{} {
	t.Helper()
	names := mapping.Biome419Names()
	valid := make(map[uint32]struct{}, len(names))
	for _, id := range names {
		valid[id] = struct{}{}
	}
	return valid
}

// TestBiome419MatchesLockedDefinitions holds the mapping's name table against
// the locked 1.16.100 biome definition snapshot: every name the client knows
// must appear with the ID this table claims for it, and nothing else. The
// client resolves per-column biome bytes against this exact registry, so an
// entry that drifted - or a blob entry the table missed - would surface on a
// real client as a crash while it applies the world.
func TestBiome419MatchesLockedDefinitions(t *testing.T) {
	decoded := map[string]any{}
	if err := nbt.NewDecoder(bytes.NewReader(v419.BiomeDefinitions())).Decode(&decoded); err != nil {
		t.Fatalf("decode locked biome definitions: %v", err)
	}
	names := mapping.Biome419Names()
	if len(decoded) != len(names) {
		t.Errorf("locked snapshot has %v biomes, mapping table has %v: the two must name the same set", len(decoded), len(names))
	}
	for name := range decoded {
		if _, ok := names[name]; !ok {
			t.Errorf("biome %q is in the locked snapshot but not in the mapping table", name)
		}
	}
	for name := range names {
		if _, ok := decoded[name]; !ok {
			t.Errorf("biome %q is in the mapping table but not in the locked snapshot", name)
		}
	}
}

// TestBiome419IsTotal verifies every output is an ID the 1.16.100 client can
// resolve: identity for the known set, the tabled stand-in for every biome
// newer than 1.16.100, and plains for anything unrecognised.
func TestBiome419IsTotal(t *testing.T) {
	valid := biome419ValidIDs(t)
	for name, id := range mapping.Biome419Names() {
		if mapped, ok := mapping.Biome419(id); !ok || mapped != id {
			t.Errorf("known biome %q (ID %v) should map to itself, got %v (ok=%v)", name, id, mapped, ok)
		}
	}
	// Every biome the native registry holds beyond the 71 the client knows,
	// by the runtime IDs the 26.45 registry assigns them.
	for id := uint32(178); id <= 195; id++ {
		mapped, ok := mapping.Biome419(id)
		if !ok {
			t.Errorf("biome ID %v should be remapped, not reported unmappable", id)
		}
		if _, known := valid[mapped]; !known {
			t.Errorf("biome ID %v remapped to %v, which the 419 client cannot resolve", id, mapped)
		}
	}
	// An ID outside every registry - a custom biome registered by a world.
	if mapped, ok := mapping.Biome419(4000); !ok || mapped != 1 {
		t.Errorf("unknown biome ID should fall back to plains (1), got %v (ok=%v)", mapped, ok)
	}
	// Nothing in a plausible ID range may produce a byte the client cannot resolve.
	for id := uint32(0); id <= 256; id++ {
		mapped, _ := mapping.Biome419(id)
		if _, known := valid[mapped]; !known {
			t.Fatalf("biome ID %v mapped to %v, which the 419 client cannot resolve", id, mapped)
		}
	}
}

// TestBiome419RemapTargetsAreKnownBiomes pins the family stand-ins the table
// was designed around, so a future registry shift that silently turns a
// stand-in into an unknown ID is caught here rather than on a client.
func TestBiome419RemapTargetsAreKnownBiomes(t *testing.T) {
	names := mapping.Biome419Names()
	expected := map[uint32]uint32{
		178: names["hell"],
		179: names["hell"],
		180: names["hell"],
		181: names["hell"],
		182: names["ice_mountains"],
		183: names["ice_mountains"],
		184: names["ice_plains"],
		185: names["cold_taiga"],
		186: names["plains"],
		187: names["plains"],
		188: names["plains"],
		189: names["extreme_hills"],
		190: names["plains"],
		191: names["swampland"],
		192: names["flower_forest"],
		193: names["roofed_forest"],
		194: names["plains"],
		195: names["forest"],
	}
	for from, to := range expected {
		if mapped, _ := mapping.Biome419(from); mapped != to {
			t.Errorf("biome ID %v should map to %v, got %v", from, to, mapped)
		}
	}
}
