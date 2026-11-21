package v1_16_100

import (
	"testing"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// TestUpdateSubChunkBlocksExpandsToPerBlockUpdates covers the protocol-419
// replacement for the sub-chunk batch packet: 1.16.100 predates the sub-chunk
// request system, so a batch has to reach the client as UpdateBlock packets.
func TestUpdateSubChunkBlocksExpandsToPerBlockUpdates(t *testing.T) {
	runtime, err := newRuntimeData(protocol419IntegrationBlockRegistry(t), []protocol.ItemEntry{{Name: "minecraft:stone", RuntimeID: 1}}, MappingOptions{})
	if err != nil {
		t.Fatal(err)
	}
	p := New().(*Protocol)
	p.runtime = runtime
	target, exact := runtime.blocks.NativeToTarget(1)
	if !exact {
		t.Fatal("native runtime ID 1 has no exact protocol-419 state in the fixture registry")
	}
	input := &packet.UpdateSubChunkBlocks{
		Position: protocol.BlockPos{0, 64, 0},
		Blocks: []protocol.BlockChangeEntry{
			{BlockPos: protocol.BlockPos{1, 64, 2}, BlockRuntimeID: 1, Flags: packet.BlockUpdateNetwork},
		},
		Extra: []protocol.BlockChangeEntry{
			{BlockPos: protocol.BlockPos{1, 64, 2}, BlockRuntimeID: 1, Flags: packet.BlockUpdateNetwork},
		},
	}
	mapped := p.convertGameplayFromLatest(input, nil)
	if len(mapped) != 2 {
		t.Fatalf("target conversion count: got %d, want 2", len(mapped))
	}
	primaryTarget, ok := mapped[0].(*packet.UpdateBlock)
	if !ok {
		t.Fatalf("target packet type: got %T", mapped[0])
	}
	if primaryTarget.NewBlockRuntimeID != target || primaryTarget.Layer != 0 {
		t.Fatalf("target block update: %#v, want runtime ID %d on layer 0", primaryTarget, target)
	}
	if secondaryTarget, ok := mapped[1].(*packet.UpdateBlock); !ok || secondaryTarget.Layer != 1 {
		t.Fatalf("target second-layer update: %#v", mapped[1])
	}
	converted := p.ConvertFromLatest(input, nil)
	if len(converted) != 2 {
		t.Fatalf("conversion count: got %d, want 2", len(converted))
	}
	for index, convertedPacket := range converted {
		if convertedPacket.ID() != packet.IDUpdateBlock {
			t.Fatalf("packet %d ID: got %d, want %d", index, convertedPacket.ID(), packet.IDUpdateBlock)
		}
	}
	first := p.ConvertToLatest(converted[0], nil)
	if len(first) != 1 {
		t.Fatalf("first block update conversion count: got %d, want 1", len(first))
	}
	primary, ok := first[0].(*packet.UpdateBlock)
	if !ok {
		t.Fatalf("first block update type: got %T", first[0])
	}
	if primary.Position != (protocol.BlockPos{1, 64, 2}) || primary.NewBlockRuntimeID != 1 || primary.Layer != 0 {
		t.Fatalf("first block update: %#v", primary)
	}
	second := p.ConvertToLatest(converted[1], nil)
	if len(second) != 1 {
		t.Fatalf("second block update conversion count: got %d, want 1", len(second))
	}
	secondary, ok := second[0].(*packet.UpdateBlock)
	if !ok {
		t.Fatalf("second block update type: got %T", second[0])
	}
	if secondary.Layer != 1 {
		t.Fatalf("second block update layer: got %d, want 1", secondary.Layer)
	}
	if len(input.Blocks) != 1 || input.Blocks[0].BlockRuntimeID != 1 || len(input.Extra) != 1 || input.Extra[0].BlockRuntimeID != 1 {
		t.Fatalf("conversion mutated the native batch: %#v", input)
	}
}
