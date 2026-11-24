package v1_18_10

import (
	"bytes"
	"testing"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
)

// TestLargeShapedRecipeGridDecodes decodes the biggest grid a real crafting
// table uses (3x3) through the guarded shaped-recipe path, so the per-recipe
// dimension check is shown to accept every legitimate recipe size.
func TestLargeShapedRecipeGridDecodes(t *testing.T) {
	recipe := &protocol.ShapedRecipe{
		RecipeID: "grid",
		Width:    3,
		Height:   3,
		Input:    make([]protocol.ItemDescriptorCount, 9),
	}
	var buffer bytes.Buffer
	marshalShapedRecipeV486(newWireIO(protocol.NewWriter(&buffer, 0), false, nil), recipe)
	decoded := &protocol.ShapedRecipe{Width: 3, Height: 3}
	marshalShapedRecipeV486(newWireIO(protocol.NewReader(bytes.NewBuffer(buffer.Bytes()), 0, true), true, nil), decoded)
	if len(decoded.Input) != 9 {
		t.Fatalf("recipe grid entries: got %d, want 9", len(decoded.Input))
	}
}
