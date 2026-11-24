package v1_16_100

import (
	"sync"
	"testing"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"github.com/shawtymarco/go-multiversion/mapping"
)

// TestUnmappedSubChunkBlockStillSendsAndLogsOnce proves the placeholder policy:
// a block the 419 palette cannot represent still reaches the client as the air
// fallback, and the missing mapping is reported once for repeated entries
// rather than once per packet.
func TestUnmappedSubChunkBlockStillSendsAndLogsOnce(t *testing.T) {
	registry := protocol419IntegrationBlockRegistry(t)
	registry.states = append(registry.states, mapping.BlockState{Name: "minecraft:fallback_test_block"})
	unmapped := uint32(len(registry.states) - 1)
	runtime, err := newRuntimeData(registry, []protocol.ItemEntry{{Name: "minecraft:stone", RuntimeID: 1}}, MappingOptions{})
	if err != nil {
		t.Fatal(err)
	}

	mapping.ResetFallbackReports()
	var mu sync.Mutex
	var reports []mapping.Fallback
	previous := mapping.SetFallbackLogger(func(f mapping.Fallback) {
		mu.Lock()
		reports = append(reports, f)
		mu.Unlock()
	})
	defer mapping.SetFallbackLogger(previous)

	air, exact := runtime.blocks.NativeToTarget(unmapped)
	if exact || air != runtime.blocks.TargetAir() {
		t.Fatalf("unmapped block: got %d exact=%v, want air %d/false", air, exact, runtime.blocks.TargetAir())
	}

	positions := []protocol.BlockPos{{-1234, -30, 5678}, {-1230, -30, 5678}, {-1234, -28, 5678}}
	batch := &packet.UpdateSubChunkBlocks{Position: protocol.BlockPos{-1248, -32, 5664}}
	for _, position := range positions {
		batch.Blocks = append(batch.Blocks, protocol.BlockChangeEntry{BlockPos: position, BlockRuntimeID: unmapped, Flags: packet.BlockUpdateNetwork})
	}
	sent := targetBlockChanges(batch, runtime.blocks)
	if len(sent) != len(positions) {
		t.Fatalf("sent packets: got %d, want %d", len(sent), len(positions))
	}
	for index, raw := range sent {
		update, ok := raw.(*packet.UpdateBlock)
		if !ok {
			t.Fatalf("packet %d: got %T, want *packet.UpdateBlock", index, raw)
		}
		if update.NewBlockRuntimeID != air || update.Position != positions[index] {
			t.Fatalf("packet %d: %#v, want runtime ID %d at %v", index, update, air, positions[index])
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(reports) != 1 {
		t.Fatalf("fallback reports: got %d (%#v), want 1", len(reports), reports)
	}
	want := mapping.Fallback{Protocol: ID, Kind: "block", Name: "minecraft:fallback_test_block", Used: "minecraft:air"}
	if reports[0] != want {
		t.Fatalf("report: got %#v, want %#v", reports[0], want)
	}
}
