package chunkconv

import (
	"bytes"
	"fmt"
	"testing"
)

// benchmarkPaletteSizes are the palette sizes the chunk encoder produces in
// practice: uniform (1), mixed terrain (4), richly varied terrain (16) and a
// fully populated 8-bit palette (256).
var benchmarkPaletteSizes = []int{1, 4, 16, 256}

func paletteBits(entries int) int {
	switch {
	case entries <= 1:
		return 0
	case entries <= 4:
		return 2
	case entries <= 16:
		return 4
	default:
		return 8
	}
}

// benchSubChunk builds one version-9 sub-chunk whose palette has entries
// distinct values. The index words cycle through every palette entry so the
// repack path touches all of them.
func benchSubChunk(entries int) []byte {
	bits := paletteBits(entries)
	values := make([]uint32, entries)
	for i := range values {
		values[i] = uint32(4096 + i)
	}
	source := append([]byte{9, 1, 0}, fixturePalette(bits, values)...)
	return append(source, 10, 0, 0, 8, 1, 2, 3, 0)
}

// benchLevelChunk builds a full overworld chunk: 24 sub-chunks plus 24 biome
// sections, each with its own palette.
func benchLevelChunk(entries int) []byte {
	out := new(bytes.Buffer)
	section := benchSubChunk(entries)
	section = section[:len(section)-8]
	for range 24 {
		out.Write(section)
	}
	for i := range 24 {
		if i == 0 {
			out.Write(fixturePalette(0, []uint32{195}))
			continue
		}
		out.WriteByte(255)
	}
	out.Write([]byte{0, 10, 0, 42})
	return out.Bytes()
}

func BenchmarkSubChunk(b *testing.B) {
	// remap mirrors a historical registry whose IDs shift by a constant, so the
	// mapping is injective and the palette keeps its width.
	remap := func(id uint32) (uint32, bool) { return id + 7, true }
	for _, entries := range benchmarkPaletteSizes {
		source := benchSubChunk(entries)
		b.Run(fmt.Sprintf("entries=%d/identity", entries), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			b.ResetTimer()
			for range b.N {
				if _, err := SubChunk(source, identity); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(fmt.Sprintf("entries=%d/remap", entries), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			b.ResetTimer()
			for range b.N {
				if _, err := SubChunk(source, remap); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkLevelChunk(b *testing.B) {
	biomes := func(id uint32) (uint32, bool) {
		if id == 195 {
			return 4, true
		}
		return id, true
	}
	for _, entries := range benchmarkPaletteSizes {
		source := benchLevelChunk(entries)
		b.Run(fmt.Sprintf("entries=%d", entries), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			b.ResetTimer()
			for range b.N {
				if _, err := LevelChunk(source, 24, 24, identity, biomes); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
