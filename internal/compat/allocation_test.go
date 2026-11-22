package compat

import (
	"bytes"
	"testing"

	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// TestOversizedWireLengthsRejected proves that a wire-supplied element count is
// checked before it is used as an allocation size. The SubChunk decoder used to
// allocate from the raw uint32 count, so a 13-byte payload made it request a
// 1.9 TB slice and killed the process with a fatal out-of-memory error instead
// of reporting a protocol error.
//
// Packet IDs are not the same on every version, so each case converts a real
// packet to learn the target ID instead of assuming the native one.
func TestOversizedWireLengthsRejected(t *testing.T) {
	cases := []struct {
		name     string
		listener bool
		payload  []byte
		targetID func(p minecraft.Protocol) (uint32, bool)
	}{
		{
			name:     "sub_chunk",
			listener: false,
			// cache-enabled bool, dimension, three position varints, then a
			// uint32 entry count of 0xffffffff with no entries behind it.
			payload: []byte{1, 0, 0, 0, 0, 0xff, 0xff, 0xff, 0xff},
			targetID: func(p minecraft.Protocol) (uint32, bool) {
				out := p.ConvertFromLatest(&packet.SubChunk{}, nil)
				if len(out) == 0 {
					return 0, false
				}
				return out[0].ID(), true
			},
		},
		{
			name:     "resource_pack_response",
			listener: true,
			// response byte, then a uint16 pack count of 0xffff with no strings
			// behind it.
			payload: []byte{2, 0xff, 0xff},
			targetID: func(p minecraft.Protocol) (uint32, bool) {
				out := p.ConvertToLatest(&packet.ResourcePackClientResponse{
					Response:        1,
					PacksToDownload: []string{"pack"},
				}, nil)
				if len(out) == 0 {
					return 0, false
				}
				return out[0].ID(), true
			},
		},
	}

	for _, p := range catalogue(t) {
		for _, test := range cases {
			id, ok := test.targetID(p)
			if !ok {
				continue
			}
			constructor, ok := p.Packets(test.listener)[id]
			if !ok {
				continue
			}
			t.Run(test.name, func(t *testing.T) {
				defer func() {
					recovered := recover()
					if recovered == nil {
						t.Fatalf("protocol %d accepted a %d-byte %s payload with an oversized length", p.ID(), len(test.payload), test.name)
					}
					if fatal := fatalPanic(recovered); fatal != nil {
						t.Fatalf("protocol %d crashed instead of reporting a protocol error: %v", p.ID(), fatal)
					}
				}()
				decoded := constructor()
				decoded.Marshal(p.NewReader(bytes.NewReader(test.payload), 0, test.listener))
			})
		}
	}
}
