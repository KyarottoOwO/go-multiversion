package compat

import (
	"bytes"
	"testing"

	"github.com/sandertv/gophertunnel/minecraft"
)

// FuzzAdapterDecode feeds arbitrary bytes into every adapter decoder, keyed by
// the protocol, direction and packet ID. Decoders report malformed input by
// panicking with an error, which the connection loop recovers; a runtime panic
// (index out of range, nil dereference) would take the process down instead, so
// only those fail the fuzzer.
func FuzzAdapterDecode(f *testing.F) {
	fixtures := catalogue(f)
	for _, p := range fixtures {
		for _, listener := range []bool{false, true} {
			for id, constructor := range p.Packets(listener) {
				var body bytes.Buffer
				func() {
					defer func() { _ = recover() }()
					constructor().Marshal(p.NewWriter(&body, 0))
				}()
				f.Add(uint32(p.ID()), listener, id, body.Bytes())
			}
		}
	}
	f.Fuzz(func(t *testing.T, protocolID uint32, listener bool, id uint32, data []byte) {
		var target minecraft.Protocol
		for _, p := range fixtures {
			if uint32(p.ID()) == protocolID {
				target = p
				break
			}
		}
		if target == nil {
			t.Skip()
		}
		constructor, ok := target.Packets(listener)[id]
		if !ok {
			t.Skip()
		}
		defer func() {
			if recovered := recover(); fatalPanic(recovered) != nil {
				t.Fatalf("protocol %d packet %d panicked on %d bytes: %v", protocolID, id, len(data), recovered)
			}
		}()
		decoded := constructor()
		decoded.Marshal(target.NewReader(bytes.NewReader(data), 0, listener))
	})
}

// FuzzConversionRoundTrip decodes target bytes, converts them to the native
// model and converts the result back to the target. Whatever a hostile client
// manages to decode, the adapter must keep converting without a runtime panic.
func FuzzConversionRoundTrip(f *testing.F) {
	fixtures := catalogue(f)
	for _, p := range fixtures {
		for _, input := range samples(f) {
			converted := p.ConvertFromLatest(input.pk, nil)
			for _, out := range converted {
				var wire bytes.Buffer
				func() {
					defer func() { _ = recover() }()
					out.Marshal(p.NewWriter(&wire, 0))
				}()
				f.Add(uint32(p.ID()), uint32(out.ID()), wire.Bytes())
			}
		}
	}
	f.Fuzz(func(t *testing.T, protocolID uint32, packetID uint32, data []byte) {
		var target minecraft.Protocol
		for _, p := range fixtures {
			if uint32(p.ID()) == protocolID {
				target = p
				break
			}
		}
		if target == nil {
			t.Skip()
		}
		constructor, ok := target.Packets(true)[packetID]
		if !ok {
			t.Skip()
		}
		defer func() {
			if recovered := recover(); fatalPanic(recovered) != nil {
				t.Fatalf("protocol %d packet %d panicked: %v", protocolID, packetID, recovered)
			}
		}()
		decoded := constructor()
		decoded.Marshal(target.NewReader(bytes.NewReader(data), 0, false))
		for _, latest := range target.ConvertToLatest(decoded, nil) {
			if latest == nil {
				t.Fatalf("protocol %d packet %d converted to nil", protocolID, packetID)
			}
			for _, back := range target.ConvertFromLatest(latest, nil) {
				if back != nil {
					var sink bytes.Buffer
					back.Marshal(target.NewWriter(&sink, 0))
				}
			}
		}
	})
}
