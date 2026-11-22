package compat

import (
	"bytes"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"github.com/shawtymarco/go-multiversion/chunkconv"
)

// The targets below split the conversion surface into the areas a single
// decode/convert fuzzer covers only shallowly: chunk palettes, login and
// compression negotiation, the limits-enabled decode path used for batched
// client traffic, recipes and creative content, and packets with nested
// structures. As in fuzz_test.go, only runtime panics (index out of range, nil
// dereference, impossible allocation) fail a target; gophertunnel reports
// malformed input as a panicked error.

// findProtocol returns the catalogue entry with the given protocol ID.
func findProtocol(list []minecraft.Protocol, id uint32) minecraft.Protocol {
	for _, p := range list {
		if uint32(p.ID()) == id {
			return p
		}
	}
	return nil
}

// adapterID asks the adapter which wire ID it uses for a native packet, so a
// target never assumes that the native ID is the target ID.
func adapterID(p minecraft.Protocol, nativePk packet.Packet) (uint32, bool) {
	converted := p.ConvertFromLatest(nativePk, nil)
	if len(converted) == 0 {
		return 0, false
	}
	return converted[0].ID(), true
}

// seedWire adds the adapter's wire form of a native packet to the corpus.
func seedWire(f *testing.F, p minecraft.Protocol, nativePk packet.Packet, serverbound bool) {
	id, ok := adapterID(p, nativePk)
	if !ok {
		return
	}
	var body bytes.Buffer
	func() {
		defer func() { _ = recover() }()
		nativePk.Marshal(p.NewWriter(&body, 0))
	}()
	f.Add(uint32(p.ID()), id, serverbound, body.Bytes())
}

// fuzzPackets are the native packets each surface target starts from.
func nestedPackets() []packet.Packet {
	stone := protocol.ItemStack{ItemType: protocol.ItemType{NetworkID: 1}, Count: 1}
	return []packet.Packet{
		&packet.PlayerAuthInput{
			Tick: 20, Position: mgl32.Vec3{1, 64, 3}, Pitch: 10, Yaw: 20,
			ItemInteractionData: protocol.Option(protocol.UseItemTransactionData{
				ActionType: protocol.UseItemActionClickBlock, BlockPosition: protocol.BlockPos{1, 63, 3},
				BlockRuntimeID: 1, HeldItem: protocol.ItemInstance{Stack: stone},
			}),
		},
		&packet.InventoryTransaction{
			TransactionData: &protocol.UseItemOnEntityTransactionData{
				TargetEntityRuntimeID: 9, ActionType: protocol.UseItemOnEntityActionInteract,
				HeldItem: protocol.ItemInstance{Stack: stone},
			},
			Actions: []protocol.InventoryAction{{
				SourceType: protocol.InventoryActionSourceContainer, InventorySlot: 1,
				OldItem: protocol.ItemInstance{Stack: stone}, NewItem: protocol.ItemInstance{},
			}},
		},
		&packet.BookEdit{InventorySlot: 1, ActionType: packet.BookActionSign, Author: "Steve", Title: "Book", XUID: "42"},
		&packet.SetScore{Entries: []protocol.ScoreboardEntry{{
			EntryID: 1, ObjectiveName: "obj", Score: 5,
			IdentityType: protocol.ScoreboardIdentityFakePlayer, DisplayName: "line",
		}}},
		&packet.ModalFormResponse{FormID: 1, ResponseData: protocol.Option([]byte(`{"a":1}`))},
	}
}

func recipePackets() []packet.Packet {
	stone := protocol.ItemStack{ItemType: protocol.ItemType{NetworkID: 1}, Count: 1}
	return []packet.Packet{
		&packet.CraftingData{
			ShapelessRecipes: []protocol.ShapelessRecipe{{
				Block: "crafting_table", RecipeID: "1",
				Input:  []protocol.ItemDescriptorCount{{Descriptor: &protocol.DefaultItemDescriptor{Name: "minecraft:stone"}, Count: 1}},
				Output: []protocol.ItemStack{stone},
			}},
			ClearRecipes: true,
		},
		&packet.CreativeContent{
			Groups: []protocol.CreativeGroup{{Name: "itemGroup.name.stone", Icon: stone}},
			Items:  []protocol.CreativeItem{{CreativeItemNetworkID: 1, Item: stone}},
		},
	}
}

func loginPackets() []packet.Packet {
	return []packet.Packet{
		&packet.Login{ClientProtocol: 2225, ConnectionRequest: []byte{1, 2, 3}},
		&packet.RequestNetworkSettings{ClientProtocol: 2225},
		&packet.NetworkSettings{CompressionThreshold: 256, CompressionAlgorithm: packet.CompressionAlgorithmFlate},
		&packet.ClientToServerHandshake{},
	}
}

// FuzzChunkPaletteDecode feeds arbitrary bytes to the version-9 palette
// translator as both a level chunk and a sub-chunk entry payload.
func FuzzChunkPaletteDecode(f *testing.F) {
	adapters := catalogue(f)
	// A valid chunk made only of empty sections and biome reuse: 24 version-9
	// sections with a one-entry palette, 24 biome sections and the border tail.
	valid := make([]byte, 0, 128)
	for range 24 {
		valid = append(valid, 9, 1, 0)
	}
	valid = append(valid, 0, 0) // first biome palette: bits 0, value 0
	for range 23 {
		valid = append(valid, 0xff)
	}
	valid = append(valid, 0, 10, 0, 42)
	validSection := []byte{9, 1, 0}
	for _, p := range adapters {
		f.Add(uint32(p.ID()), int32(packet.DimensionOverworld), uint32(24), append([]byte(nil), valid...))
		f.Add(uint32(p.ID()), int32(packet.DimensionOverworld), uint32(1), validSection)
	}
	f.Fuzz(func(t *testing.T, protocolID uint32, dimension int32, count uint32, payload []byte) {
		p := findProtocol(adapters, protocolID)
		if p == nil {
			t.Skip()
		}
		defer func() {
			if recovered := recover(); fatalPanic(recovered) != nil {
				t.Fatalf("protocol %d chunk conversion panicked: %v", protocolID, recovered)
			}
		}()
		chunk := &packet.LevelChunk{Dimension: dimension, SubChunkCount: count, RawPayload: payload}
		_, _ = chunkconv.Packet(chunk, p)
		_, _ = chunkconv.Packet(&packet.SubChunk{
			SubChunkEntries: []protocol.SubChunkEntry{{Result: protocol.SubChunkResultSuccess, RawPayload: protocol.Option(payload)}},
		}, p)
	})
}

// FuzzLoginNegotiation covers login and compression negotiation, the packets a
// client sends before any game data is exchanged.
func FuzzLoginNegotiation(f *testing.F) {
	adapters := catalogue(f)
	for _, p := range adapters {
		for _, pk := range loginPackets() {
			seedWire(f, p, pk, true)
		}
	}
	f.Fuzz(func(t *testing.T, protocolID uint32, packetID uint32, listener bool, data []byte) {
		p := findProtocol(adapters, protocolID)
		if p == nil {
			t.Skip()
		}
		constructor, ok := p.Packets(listener)[packetID]
		if !ok {
			t.Skip()
		}
		defer func() {
			if recovered := recover(); fatalPanic(recovered) != nil {
				t.Fatalf("protocol %d packet %d login decode panicked: %v", protocolID, packetID, recovered)
			}
		}()
		decoded := constructor()
		decoded.Marshal(p.NewReader(bytes.NewReader(data), 0, true))
		for _, latest := range p.ConvertToLatest(decoded, nil) {
			if latest == nil {
				t.Fatalf("protocol %d packet %d converted to nil", protocolID, packetID)
			}
			for _, back := range p.ConvertFromLatest(latest, nil) {
				if back != nil {
					var sink bytes.Buffer
					back.Marshal(p.NewWriter(&sink, 0))
				}
			}
		}
	})
}

// FuzzBatchFraming exercises the limits-enabled decode path every packet takes
// after it is split out of a compressed batch. The library owns compression
// settings, not batch framing (gophertunnel owns that), so this target checks
// that no pool entry crashes on arbitrary bytes with limits on. An empty packet
// legitimately consumes no bytes, so this target checks crashes only.
func FuzzBatchFraming(f *testing.F) {
	adapters := catalogue(f)
	for _, p := range adapters {
		for _, listener := range []bool{false, true} {
			for _, nativePk := range []packet.Packet{
				&packet.Text{TextType: packet.TextTypeChat, Message: "hello"},
				&packet.RequestChunkRadius{ChunkRadius: 8},
			} {
				id, ok := adapterID(p, nativePk)
				if !ok {
					continue
				}
				var body bytes.Buffer
				func() {
					defer func() { _ = recover() }()
					nativePk.Marshal(p.NewWriter(&body, 0))
				}()
				f.Add(uint32(p.ID()), listener, id, body.Bytes())
			}
		}
	}
	f.Fuzz(func(t *testing.T, protocolID uint32, listener bool, packetID uint32, data []byte) {
		p := findProtocol(adapters, protocolID)
		if p == nil {
			t.Skip()
		}
		constructor, ok := p.Packets(listener)[packetID]
		if !ok {
			t.Skip()
		}
		defer func() {
			if recovered := recover(); fatalPanic(recovered) != nil {
				t.Fatalf("protocol %d packet %d batched decode panicked: %v", protocolID, packetID, recovered)
			}
		}()
		reader := bytes.NewBuffer(data)
		decoded := constructor()
		decoded.Marshal(p.NewReader(reader, 0, true))
	})
}

// FuzzRecipeCreative covers the two clientbound catalogues that carry nested
// descriptors and item stacks, in both conversion directions.
func FuzzRecipeCreative(f *testing.F) {
	adapters := catalogue(f)
	for _, p := range adapters {
		for _, pk := range recipePackets() {
			seedWire(f, p, pk, false)
		}
	}
	f.Fuzz(func(t *testing.T, protocolID uint32, packetID uint32, listener bool, data []byte) {
		p := findProtocol(adapters, protocolID)
		if p == nil {
			t.Skip()
		}
		constructor, ok := p.Packets(listener)[packetID]
		if !ok {
			t.Skip()
		}
		defer func() {
			if recovered := recover(); fatalPanic(recovered) != nil {
				t.Fatalf("protocol %d packet %d recipe decode panicked: %v", protocolID, packetID, recovered)
			}
		}()
		decoded := constructor()
		decoded.Marshal(p.NewReader(bytes.NewReader(data), 0, listener))
		for _, latest := range p.ConvertToLatest(decoded, nil) {
			if latest == nil {
				t.Fatalf("protocol %d packet %d converted to nil", protocolID, packetID)
			}
			for _, back := range p.ConvertFromLatest(latest, nil) {
				if back != nil {
					var sink bytes.Buffer
					back.Marshal(p.NewWriter(&sink, 0))
				}
			}
		}
	})
}

// FuzzNestedFields covers packets whose payload contains other structures,
// such as an auth input carrying a stack request or a transaction carrying a
// typed action list.
func FuzzNestedFields(f *testing.F) {
	adapters := catalogue(f)
	for _, p := range adapters {
		for _, pk := range nestedPackets() {
			_, serverbound := pk.(*packet.PlayerAuthInput)
			serverbound = serverbound || isServerboundNested(pk)
			seedWire(f, p, pk, serverbound)
		}
	}
	f.Fuzz(func(t *testing.T, protocolID uint32, packetID uint32, listener bool, data []byte) {
		p := findProtocol(adapters, protocolID)
		if p == nil {
			t.Skip()
		}
		constructor, ok := p.Packets(listener)[packetID]
		if !ok {
			t.Skip()
		}
		defer func() {
			if recovered := recover(); fatalPanic(recovered) != nil {
				t.Fatalf("protocol %d packet %d nested decode panicked: %v", protocolID, packetID, recovered)
			}
		}()
		decoded := constructor()
		decoded.Marshal(p.NewReader(bytes.NewReader(data), 0, true))
		for _, latest := range p.ConvertToLatest(decoded, nil) {
			if latest == nil {
				t.Fatalf("protocol %d packet %d converted to nil", protocolID, packetID)
			}
			for _, back := range p.ConvertFromLatest(latest, nil) {
				if back != nil {
					var sink bytes.Buffer
					back.Marshal(p.NewWriter(&sink, 0))
				}
			}
		}
	})
}

// isServerboundNested reports whether a nested-fixture packet travels from the
// client.
func isServerboundNested(pk packet.Packet) bool {
	switch pk.(type) {
	case *packet.PlayerAuthInput, *packet.InventoryTransaction, *packet.ModalFormResponse:
		return true
	}
	return false
}
