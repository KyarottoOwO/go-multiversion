// Package chunkconv translates network chunk palettes without accessing a server's
// live world. Its input is Dragonfly's version-9 numeric network encoding. Proxies
// that keep their backends native use Packet on the way to a historical client.
package chunkconv

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
)

type Mapper func(uint32) (uint32, bool)

// SubChunk maps one version-9 sub-chunk and preserves the trailing block NBT.
func SubChunk(data []byte, blocks Mapper) ([]byte, error) {
	r, w := bytes.NewReader(data), bytes.NewBuffer(make([]byte, 0, len(data)))
	if err := subChunk(r, w, blocks); err != nil {
		return nil, err
	}
	_, _ = io.Copy(w, r)
	return w.Bytes(), nil
}

// LevelChunk maps the declared sub-chunks and all biome sections, preserving
// border blocks and block NBT verbatim. Cache blobs are deliberately not accepted.
func LevelChunk(data []byte, count uint32, sections int, blocks, biomes Mapper) ([]byte, error) {
	if sections < 1 || sections > 64 || count > uint32(sections) {
		return nil, fmt.Errorf("invalid chunk dimensions: %d sub-chunks, %d biome sections", count, sections)
	}
	r, w := bytes.NewReader(data), bytes.NewBuffer(make([]byte, 0, len(data)))
	for range count {
		if err := subChunk(r, w, blocks); err != nil {
			return nil, err
		}
	}
	for i := range sections {
		if err := palette(r, w, biomes, i > 0); err != nil {
			return nil, fmt.Errorf("biome section %d: %w", i, err)
		}
	}
	if r.Len() == 0 {
		return nil, fmt.Errorf("missing border-block count")
	}
	_, _ = io.Copy(w, r)
	return w.Bytes(), nil
}

func subChunk(r *bytes.Reader, w *bytes.Buffer, blocks Mapper) error {
	var header [3]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return err
	}
	if header[0] != 9 {
		return fmt.Errorf("unsupported sub-chunk version %d", header[0])
	}
	w.Write(header[:])
	for range header[1] {
		if err := palette(r, w, blocks, false); err != nil {
			return err
		}
	}
	return nil
}

// inlinePaletteCount is the number of distinct palette values resolved with a
// linear scan before a map takes over. Real sub-chunk palettes are small, so the
// scan avoids one map allocation per palette; larger palettes still use a map.
const inlinePaletteCount = 32

// palette translates one version-9 block or biome palette. The index words are
// walked word by word: a packed field never spans two words, so the destination
// bit position can be advanced instead of divided per cell.
func palette(r *bytes.Reader, w *bytes.Buffer, mapID Mapper, reuse bool) error {
	header, err := r.ReadByte()
	if err != nil {
		return err
	}
	if header == 255 && reuse {
		w.WriteByte(header)
		return nil
	}
	bits := int(header >> 1)
	if header&1 == 0 || !validBits(bits) {
		return fmt.Errorf("invalid network palette header %d", header)
	}
	var words []byte
	if bits != 0 {
		words = make([]byte, (4096+32/bits-1)/(32/bits)*4)
		if _, err := io.ReadFull(r, words); err != nil {
			return err
		}
	}
	count := int32(1)
	if bits != 0 {
		if err := protocol.Varint32(r, &count); err != nil {
			return err
		}
	}
	if count < 1 || count > 4096 || (bits != 0 && count > 1<<bits) {
		return fmt.Errorf("invalid palette length %d", count)
	}
	remap := make([]uint32, count)
	var (
		inlineTable [inlinePaletteCount]uint32
		values      = inlineTable[:0]
		indices     map[uint32]uint32
		identity    = true
	)
	if count > inlinePaletteCount {
		// A large palette would grow the inline table several times before the
		// map takes over. Size it exactly once instead.
		values = make([]uint32, 0, count)
	}
	for i := range remap {
		var value int32
		if err := protocol.Varint32(r, &value); err != nil {
			return err
		}
		mapped, ok := mapID(uint32(value))
		if !ok {
			return fmt.Errorf("unknown palette ID %d", uint32(value))
		}
		if mapped != uint32(value) {
			identity = false
		}
		index, found := uint32(0), false
		if indices != nil {
			index, found = indices[mapped]
		} else {
			for k := range values {
				if values[k] == mapped {
					index, found = uint32(k), true
					break
				}
			}
		}
		if !found {
			index = uint32(len(values))
			values = append(values, mapped)
			switch {
			case indices != nil:
				indices[mapped] = index
			case len(values) > len(inlineTable):
				// The inline table overflowed once. Keep resolving through a map
				// rather than scanning a long slice for every remaining entry.
				indices = make(map[uint32]uint32, len(values))
				for k, existing := range values {
					indices[existing] = uint32(k)
				}
			}
		}
		remap[i] = index
	}
	newBits := 0
	for _, candidate := range []int{0, 1, 2, 3, 4, 5, 6, 8, 16} {
		if len(values) <= 1<<candidate {
			newBits = candidate
			break
		}
	}
	// An identity remap that keeps the width changes no cell and no palette
	// entry, so the bytes already read are the bytes to emit. Out-of-range cell
	// indices must still be rejected exactly as the repacking path rejects them.
	if identity && len(values) == int(count) && newBits == bits {
		if bits != 0 && int(count) != 1<<uint(bits) {
			if err := validatePaletteCells(words, bits, 0, uint32(len(remap))); err != nil {
				return err
			}
		}
		w.WriteByte(header)
		w.Write(words)
		if bits != 0 {
			_ = protocol.WriteVarint32(w, count)
		}
		for _, value := range values {
			_ = protocol.WriteVarint32(w, int32(value))
		}
		return nil
	}
	var packed []uint32
	if newBits != 0 {
		packed = make([]uint32, (4096+32/newBits-1)/(32/newBits))
	}
	if bits != 0 {
		mask := uint32(1)<<uint(bits) - 1
		cellsPerWord := 32 / bits
		outWord, outOffset, cell := 0, uint(0), 0
		for offset := 0; offset < len(words) && cell < 4096; offset += 4 {
			word := binary.LittleEndian.Uint32(words[offset:])
			for k := 0; k < cellsPerWord && cell < 4096; k++ {
				old := word >> (uint(k) * uint(bits)) & mask
				if old >= uint32(len(remap)) {
					return fmt.Errorf("palette index %d outside length %d", old, len(remap))
				}
				if newBits != 0 {
					packed[outWord] |= remap[old] << outOffset
					outOffset += uint(newBits)
					if outOffset+uint(newBits) > 32 {
						outWord, outOffset = outWord+1, 0
					}
				}
				cell++
			}
		}
	}
	w.WriteByte(byte(newBits<<1 | 1))
	_ = binary.Write(w, binary.LittleEndian, packed)
	if newBits != 0 {
		_ = protocol.WriteVarint32(w, int32(len(values)))
	}
	for _, value := range values {
		_ = protocol.WriteVarint32(w, int32(value))
	}
	return nil
}

// validatePaletteCells reports the first cell index that the palette cannot
// represent. Cells stored in the padding bits after the last real cell of a
// padded palette (3, 5 and 6 bits per index) are not part of the payload.
func validatePaletteCells(words []byte, bits int, first, limit uint32) error {
	mask := uint32(1)<<uint(bits) - 1
	cellsPerWord := 32 / bits
	cell := first
	for offset := 0; offset < len(words) && cell < 4096; offset += 4 {
		word := binary.LittleEndian.Uint32(words[offset:])
		for k := 0; k < cellsPerWord && cell < 4096; k++ {
			if old := word >> (uint(k) * uint(bits)) & mask; old >= limit {
				return fmt.Errorf("palette index %d outside length %d", old, limit)
			}
			cell++
		}
	}
	return nil
}

func validBits(bits int) bool {
	switch bits {
	case 0, 1, 2, 3, 4, 5, 6, 8, 16:
		return true
	}
	return false
}
