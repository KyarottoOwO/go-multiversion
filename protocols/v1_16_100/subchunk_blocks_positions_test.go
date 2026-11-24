package v1_16_100

import (
	"testing"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// TestUpdateSubChunkBlocksPreservesAbsolutePositions pins the coordinate space
// of the protocol-419 expansion. Entry positions are absolute world
// coordinates, so they must survive unchanged - including negative values, Y
// below zero and Y above 256 - and must not be rebased onto the packet's
// sub-chunk position.
func TestUpdateSubChunkBlocksPreservesAbsolutePositions(t *testing.T) {
	runtime, err := newRuntimeData(protocol419IntegrationBlockRegistry(t), []protocol.ItemEntry{{Name: "minecraft:stone", RuntimeID: 1}}, MappingOptions{})
	if err != nil {
		t.Fatal(err)
	}
	p := New().(*Protocol)
	p.runtime = runtime

	base := protocol.BlockPos{-1248, -32, 5664}
	positions := []protocol.BlockPos{
		{-1234, -30, 5678},
		{-1234, 300, 5678},
		{-1230, 300, 5680},
	}
	blocks := make([]protocol.BlockChangeEntry, len(positions))
	for index, position := range positions {
		blocks[index] = protocol.BlockChangeEntry{BlockPos: position, BlockRuntimeID: 1, Flags: packet.BlockUpdateNetwork}
	}
	blocks[2].SyncedUpdateEntityUniqueID = 42
	blocks[2].SyncedUpdateType = packet.BlockToEntityTransition
	input := &packet.UpdateSubChunkBlocks{
		Position: base,
		Blocks:   blocks,
		Extra:    []protocol.BlockChangeEntry{{BlockPos: positions[0], BlockRuntimeID: 1, Flags: packet.BlockUpdateNetwork}},
	}

	mapped := p.convertGameplayFromLatest(input, nil)
	if len(mapped) != 4 {
		t.Fatalf("packet count: got %d, want 4", len(mapped))
	}
	for index := range 3 {
		position := positions[index]
		if index == 2 {
			synced, ok := mapped[index].(*packet.UpdateBlockSynced)
			if !ok {
				t.Fatalf("packet %d: got %T, want *packet.UpdateBlockSynced", index, mapped[index])
			}
			if synced.Position != position {
				t.Fatalf("synced position %d: got %v, want %v", index, synced.Position, position)
			}
			if synced.EntityUniqueID != 42 || synced.TransitionType != uint64(packet.BlockToEntityTransition) || synced.Layer != 0 {
				t.Fatalf("synced entry: %#v", synced)
			}
			continue
		}
		update, ok := mapped[index].(*packet.UpdateBlock)
		if !ok {
			t.Fatalf("packet %d: got %T, want *packet.UpdateBlock", index, mapped[index])
		}
		if update.Position != position {
			t.Fatalf("position %d: got %v, want %v", index, update.Position, position)
		}
		if update.Layer != 0 {
			t.Fatalf("layer %d: got %d, want 0", index, update.Layer)
		}
	}
	extra, ok := mapped[3].(*packet.UpdateBlock)
	if !ok {
		t.Fatalf("extra packet: got %T, want *packet.UpdateBlock", mapped[3])
	}
	if extra.Layer != 1 || extra.Position != positions[0] {
		t.Fatalf("extra entry: %#v", extra)
	}
	if input.Blocks[2].BlockPos != positions[2] || input.Extra[0].BlockPos != positions[0] {
		t.Fatalf("conversion mutated the batch: %#v", input)
	}
}

// TestUpdateSubChunkBlocksEmptyAndLargeBatches covers the boundaries: an empty
// batch produces no packets, and a maximum-size batch produces exactly one
// packet per entry.
func TestUpdateSubChunkBlocksEmptyAndLargeBatches(t *testing.T) {
	runtime, err := newRuntimeData(protocol419IntegrationBlockRegistry(t), []protocol.ItemEntry{{Name: "minecraft:stone", RuntimeID: 1}}, MappingOptions{})
	if err != nil {
		t.Fatal(err)
	}
	p := New().(*Protocol)
	p.runtime = runtime

	empty := p.convertGameplayFromLatest(&packet.UpdateSubChunkBlocks{Position: protocol.BlockPos{-16, 64, -16}}, nil)
	if len(empty) != 0 {
		t.Fatalf("empty batch produced %d packets", len(empty))
	}

	const entries = 4096
	batch := &packet.UpdateSubChunkBlocks{Position: protocol.BlockPos{0, 64, 0}, Blocks: make([]protocol.BlockChangeEntry, entries)}
	for index := range batch.Blocks {
		batch.Blocks[index] = protocol.BlockChangeEntry{
			BlockPos:       protocol.BlockPos{int32(index) - 2048, int32(index%320) - 64, int32(index) - 1024},
			BlockRuntimeID: 1,
			Flags:          packet.BlockUpdateNetwork,
		}
	}
	mapped := p.convertGameplayFromLatest(batch, nil)
	if len(mapped) != entries {
		t.Fatalf("large batch: got %d packets, want %d", len(mapped), entries)
	}
	last, ok := mapped[entries-1].(*packet.UpdateBlock)
	if !ok {
		t.Fatalf("last packet: got %T", mapped[entries-1])
	}
	if last.Position != batch.Blocks[entries-1].BlockPos {
		t.Fatalf("last position: got %v, want %v", last.Position, batch.Blocks[entries-1].BlockPos)
	}
}
