package chunkconv

import (
	"bytes"
	"strings"
	"testing"
)

// TestIdentityRemapIsByteIdentical locks the unchanged-palette fast path: when
// the mapper keeps every palette value and the width does not change, the
// produced payload must be exactly the source payload rather than a repacked
// copy. The set covers uniform, padded (3/5/6-bit) and full-width palettes.
func TestIdentityRemapIsByteIdentical(t *testing.T) {
	for _, test := range []struct {
		entries int
		bits    int
	}{
		{1, 0}, {2, 1}, {3, 2}, {7, 3}, {8, 3}, {15, 4}, {16, 4},
		{31, 5}, {32, 5}, {40, 6}, {64, 6}, {200, 8}, {256, 8},
	} {
		values := make([]uint32, test.entries)
		for i := range values {
			values[i] = uint32(500 + i)
		}
		source := append([]byte{9, 1, 0}, fixturePalette(test.bits, values)...)
		source = append(source, 10, 0, 0, 8, 1, 2, 3, 0)
		before := bytes.Clone(source)
		out, err := SubChunk(source, identity)
		if err != nil {
			t.Fatalf("%d entries at %d bits: %v", test.entries, test.bits, err)
		}
		if !bytes.Equal(source, out) {
			t.Fatalf("%d entries at %d bits: output is not byte-identical", test.entries, test.bits)
		}
		if !bytes.Equal(source, before) {
			t.Fatal("source mutated")
		}
	}
}

// TestIdentityFastPathStillRejectsBadIndex proves the fast path keeps the
// out-of-range palette index error, including the padded 6-bit layout.
func TestIdentityFastPathStillRejectsBadIndex(t *testing.T) {
	values := make([]uint32, 40)
	for i := range values {
		values[i] = uint32(500 + i)
	}
	source := append([]byte{9, 1, 0}, fixturePalette(6, values)...)
	// Overwrite the first palette word (the header byte precedes it) so cell 0
	// holds index 41, which the 40-entry palette cannot represent.
	source[4], source[5], source[6], source[7] = 41, 0, 0, 0
	if _, err := SubChunk(source, identity); err == nil || !strings.Contains(err.Error(), "outside length") {
		t.Fatalf("out-of-range index accepted: %v", err)
	}
}

// TestPaletteLargerThanInlineTable exercises the map promotion used once a
// palette holds more than inlinePaletteCount distinct values, in both mapping
// directions.
func TestPaletteLargerThanInlineTable(t *testing.T) {
	values := make([]uint32, 200)
	for i := range values {
		values[i] = uint32(1000 + i)
	}
	source := append([]byte{9, 1, 0}, fixturePalette(8, values)...)
	source = append(source, 10, 0, 0, 8, 1, 2, 3, 0)
	up := func(id uint32) (uint32, bool) { return id + 10000, true }
	down := func(id uint32) (uint32, bool) {
		if id < 10000 {
			return 0, false
		}
		return id - 10000, true
	}
	mapped, err := SubChunk(source, up)
	if err != nil {
		t.Fatalf("mapping up: %v", err)
	}
	if bytes.Equal(mapped, source) {
		t.Fatal("a shifted palette was not rewritten")
	}
	back, err := SubChunk(mapped, down)
	if err != nil {
		t.Fatalf("mapping down: %v", err)
	}
	if !bytes.Equal(source, back) {
		t.Fatal("palette did not round trip through the promoted table")
	}
}
