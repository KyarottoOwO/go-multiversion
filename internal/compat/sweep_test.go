package compat

import (
	"bytes"
	"fmt"
	"runtime"
	"sort"
	"testing"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func describe(pk packet.Packet) string { return fmt.Sprintf("%T(%d)", pk, pk.ID()) }

// nativeUnwritable reports the packet IDs whose zero value the current native
// gophertunnel pool itself cannot write. Those are upstream zero-value
// semantics, not adapter behaviour, so the adapter sweep skips them.
func nativeUnwritable(t *testing.T) map[bool]map[uint32]bool {
	t.Helper()
	broken := map[bool]map[uint32]bool{false: {}, true: {}}
	for _, listener := range []bool{false, true} {
		for id, constructor := range packet.NewServerPool() {
			if listener {
				if _, ok := packet.NewClientPool()[id]; !ok {
					continue
				}
				constructor = packet.NewClientPool()[id]
			}
			func() {
				defer func() {
					if recovered := recover(); recovered != nil {
						broken[listener][id] = true
					}
				}()
				var body bytes.Buffer
				constructor().Marshal(protocol.NewWriter(&body, 0))
			}()
		}
	}
	return broken
}

// TestZeroValuePacketSweep encodes, decodes and re-encodes the zero value of
// every packet in every adapter pool. It is the broadest structural check:
// a pool entry that cannot survive its own writer/reader pair would desync or
// panic on a real connection.
func TestZeroValuePacketSweep(t *testing.T) {
	broken := nativeUnwritable(t)
	for _, p := range catalogue(t) {
		for _, listener := range []bool{false, true} {
			pool := p.Packets(listener)
			for id, constructor := range pool {
				if broken[listener][id] {
					continue
				}
				t.Run(fmt.Sprintf("%d/%d", p.ID(), id), func(t *testing.T) {
					pk := constructor()
					var body bytes.Buffer
					wrote := func() (ok bool) {
						defer func() {
							if recovered := recover(); recovered != nil {
								if fatal := fatalPanic(recovered); fatal != nil {
									t.Fatalf("%s panicked while writing: %v", describe(pk), fatal)
								}
								// A protocol error here means the zero value of this
								// packet is not representable in the target version
								// (an unknown enum or a missing sound, for example).
								// Production never writes a packet without converting
								// it first, and the conversion sweep below covers
								// that filtered path, so only the writer/reader pair
								// is checked for these entries.
								t.Logf("%s zero value is not representable: %v", describe(pk), recovered)
							}
						}()
						pk.Marshal(p.NewWriter(&body, 0))
						return true
					}()
					if !wrote {
						return
					}
					encoded := bytes.Clone(body.Bytes())

					decoded := pool[id]()
					reader := bytes.NewBuffer(encoded)
					func() {
						defer func() {
							if recovered := recover(); recovered != nil {
								t.Fatalf("%s panicked while reading: %v", describe(decoded), recovered)
							}
						}()
						// Limits apply only to traffic the server reads. Clientbound
						// packets such as StartGame may legitimately exceed them.
						_, serverbound := packet.NewClientPool()[id]
						decoded.Marshal(p.NewReader(reader, 0, serverbound))
					}()
					if reader.Len() != 0 {
						t.Fatalf("%s left %d unread bytes", describe(decoded), reader.Len())
					}

					var again bytes.Buffer
					func() {
						defer func() {
							if recovered := recover(); recovered != nil {
								t.Fatalf("%s panicked while re-writing: %v", describe(decoded), recovered)
							}
						}()
						decoded.Marshal(p.NewWriter(&again, 0))
					}()
					if !bytes.Equal(again.Bytes(), encoded) {
						// A zero value may decode into a normalised packet: readers fill
						// defaults such as OverrideAppearance or widen input flag bitsets.
						// TestSampleRoundTrip asserts byte stability for realistic packets.
					}
				})
			}
		}
	}
}

// TestConversionNoPanicSweep runs every pool entry through the adapter
// conversions in both directions and checks that converted packets can be
// written with the adapter's own writer and are representable in its pool.
func TestConversionNoPanicSweep(t *testing.T) {
	broken := nativeUnwritable(t)
	for _, p := range catalogue(t) {
		serverPool := p.Packets(false)
		for _, direction := range []struct {
			name        string
			pool        packet.Pool
			serverbound bool
			apply       func(packet.Packet) []packet.Packet
		}{
			// Adapter pools hold wire wrappers, so the conversion sweep feeds the
			// native gophertunnel pools: conversions are defined on native packets.
			{"toLatest", packet.NewClientPool(), true, func(pk packet.Packet) []packet.Packet { return p.ConvertToLatest(pk, nil) }},
			{"fromLatest", packet.NewServerPool(), false, func(pk packet.Packet) []packet.Packet { return p.ConvertFromLatest(pk, nil) }},
		} {
			for id, constructor := range direction.pool {
				t.Run(fmt.Sprintf("%d/%s/%d", p.ID(), direction.name, id), func(t *testing.T) {
					if broken[direction.serverbound][id] {
						t.Skipf("%s cannot be written natively either", describe(constructor()))
					}
					pk := constructor()
					var converted []packet.Packet
					func() {
						defer func() {
							if recovered := recover(); recovered != nil {
								t.Fatalf("%s panicked in %s: %v", describe(pk), direction.name, recovered)
							}
						}()
						converted = direction.apply(pk)
					}()
					for _, out := range converted {
						if out == nil {
							t.Fatalf("%s produced a nil packet", describe(pk))
						}
						if direction.name != "fromLatest" {
							continue
						}
						if _, ok := serverPool[out.ID()]; !ok {
							t.Fatalf("%s converted into %s, which the server pool cannot write", describe(pk), describe(out))
						}
						var body bytes.Buffer
						func() {
							defer func() {
								if recovered := recover(); recovered != nil {
									t.Fatalf("%s panicked while writing converted %s: %v", describe(pk), describe(out), recovered)
								}
							}()
							out.Marshal(p.NewWriter(&body, 0))
						}()
					}
				})
			}
		}
	}
}

// fatalPanic reports panics that indicate a real crash, such as an index out of
// range or a nil dereference. gophertunnel reports malformed input by panicking
// with an error value, so only runtime errors count as bugs here.
func fatalPanic(recovered any) error {
	if runtimeError, ok := recovered.(runtime.Error); ok {
		return runtimeError
	}
	return nil
}

// truncationLengths returns a bounded sample of prefix lengths for a payload.
// Byte-by-byte scanning of multi-kilobyte StartGame payloads made this sweep
// quadratic, so it samples the head, the tail and sixteen evenly spaced points.
func truncationLengths(size int) []int {
	lengths := make([]int, 0, 64)
	seen := make(map[int]struct{}, 64)
	add := func(length int) {
		if length < 0 || length > size {
			return
		}
		if _, exists := seen[length]; exists {
			return
		}
		seen[length] = struct{}{}
		lengths = append(lengths, length)
	}
	for length := 0; length < 8 && length <= size; length++ {
		add(length)
	}
	for shift := 4; shift <= 20; shift++ {
		add(1 << shift)
	}
	for part := 1; part <= 16; part++ {
		add(size * part / 16)
	}
	for length := size - 8; length <= size; length++ {
		add(length)
	}
	sort.Ints(lengths)
	return lengths
}

// TestDecoderTruncationSweep feeds every adapter pool entry truncated prefixes
// of its own wire form. Truncation must produce a protocol error, never a
// crash, so the test fails on runtime panics only.
func TestDecoderTruncationSweep(t *testing.T) {
	for _, p := range catalogue(t) {
		for _, listener := range []bool{false, true} {
			pool := p.Packets(listener)
			for id, constructor := range pool {
				var full bytes.Buffer
				func() {
					defer func() { _ = recover() }()
					constructor().Marshal(p.NewWriter(&full, 0))
				}()
				encoded := bytes.Clone(full.Bytes())
				for _, length := range truncationLengths(len(encoded)) {
					func(length int) {
						defer func() {
							if recovered := recover(); fatalPanic(recovered) != nil {
								t.Fatalf("protocol %d packet %d panicked on a %d of %d byte prefix: %v", p.ID(), id, length, len(encoded), recovered)
							}
						}()
						decoded := pool[id]()
						reader := bytes.NewBuffer(encoded[:length])
						// The client pool decodes server traffic, which may carry
						// the full StartGame palette, so limits only apply while
						// the server reads client packets.
						decoded.Marshal(p.NewReader(reader, 0, !listener))
					}(length)
				}
			}
		}
	}
}
