package v1_26_50

import (
	"reflect"
	"testing"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	v2193 "github.com/shawtymarco/go-multiversion/data/v2193"
)

// TestBlockDefinitionsAreDecodedOnce locks the per-join cache: StartGame
// conversion must reuse one decode of the frozen vanilla components and that
// decode must stay identical to an independent decode of the embedded NBT.
func TestBlockDefinitionsAreDecodedOnce(t *testing.T) {
	first, err := blockDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) == 0 {
		t.Fatal("no block definitions decoded")
	}
	second, err := blockDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	if &first[0] != &second[0] {
		t.Fatal("block definitions were decoded again instead of reusing the cache")
	}
	fresh, err := v2193.BlockDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, fresh) {
		t.Fatal("cached block definitions differ from the embedded snapshot")
	}
}

// TestStartGameBlocksPreserveOrderAndIsolation covers both branches of the
// StartGame block assembly: the shared vanilla-only slice and the copied
// vanilla-plus-custom slice.
func TestStartGameBlocksPreserveOrderAndIsolation(t *testing.T) {
	definitions, err := blockDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	vanilla := []protocol.BlockEntry{{Name: "minecraft:stone"}, {Name: "minecraft:dirt"}}

	shared := startGameBlocks(vanilla, definitions)
	if len(shared) != len(definitions) {
		t.Fatalf("vanilla-only StartGame kept %d blocks, want %d", len(shared), len(definitions))
	}
	if &shared[0] != &definitions[0] {
		t.Fatal("vanilla-only StartGame copied the shared block definitions")
	}

	custom := protocol.BlockEntry{Name: "example:custom_block", Properties: map[string]any{"example:variant": int32(1)}}
	blocks := startGameBlocks([]protocol.BlockEntry{vanilla[0], custom, vanilla[1]}, definitions)
	if len(blocks) != len(definitions)+1 {
		t.Fatalf("StartGame kept %d blocks, want %d", len(blocks), len(definitions)+1)
	}
	if !reflect.DeepEqual(blocks[:len(definitions)], definitions) {
		t.Fatal("custom blocks displaced the vanilla component list")
	}
	if !reflect.DeepEqual(blocks[len(definitions)], custom) {
		t.Fatalf("custom block = %#v, want %#v", blocks[len(definitions)], custom)
	}
	if cap(blocks) != len(blocks) {
		t.Fatal("appending to the converted list could overwrite the shared definitions")
	}

	// A later join must still see the unmodified vanilla list even if a caller
	// appended to a previously converted slice.
	blocks = append(blocks, protocol.BlockEntry{Name: "example:second"})
	again, err := blockDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != len(definitions) || !reflect.DeepEqual(again, definitions) {
		t.Fatal("shared block definitions were mutated by a converted StartGame")
	}
}
