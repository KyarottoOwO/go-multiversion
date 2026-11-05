package mapping

import (
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"reflect"
	"testing"
)

func TestCustomItemsDoNotOverwriteVanillaOrAcceptChangedBackendIDs(t *testing.T) {
	data := map[string]any{"components": map[string]any{"item_properties": map[string]any{"minecraft:icon": map[string]any{"textures": map[string]any{"default": "example:menu"}}}}}
	custom := protocol.ItemEntry{Name: "example:menu", RuntimeID: 2, ComponentBased: true, Version: 1, Data: data}
	native := []protocol.ItemEntry{{Name: "minecraft:stone", RuntimeID: 1}, custom}
	target := map[string]TargetItem{"minecraft:stone": {RuntimeID: 2}}
	mapper, err := NewItemMapper(native, target)
	if err != nil {
		t.Fatal(err)
	}
	if id, ok := mapper.NativeToTarget(1); !ok || id != 2 {
		t.Fatal("vanilla mapping changed")
	}
	customID, ok := mapper.NativeToTarget(2)
	if !ok || customID == 2 {
		t.Fatal("custom item replaced vanilla")
	}
	if id, ok := mapper.TargetToNative(customID); !ok || id != 2 {
		t.Fatal("custom reverse mapping absent")
	}
	if len(target) != 1 {
		t.Fatal("historical snapshot mutated")
	}
	entries := mapper.RegistryEntries([]protocol.ItemEntry{custom}, 419)
	if len(entries) != 1 || entries[0].Name != custom.Name {
		t.Fatal("component-only update became a full registry")
	}
	if err := mapper.ValidateNativeEntries([]protocol.ItemEntry{custom}); err != nil {
		t.Fatal(err)
	}
	changed := custom
	changed.RuntimeID++
	if err := mapper.ValidateNativeEntries([]protocol.ItemEntry{changed}); err == nil {
		t.Fatal("another backend remapped shared state")
	}
	before := mapper.TargetEntries()
	entries[0].Data["components"].(map[string]any)["item_properties"].(map[string]any)["minecraft:icon"] = "modified"
	if !reflect.DeepEqual(before, mapper.TargetEntries()) {
		t.Fatal("target components were not isolated")
	}
	if _, ok := data["components"].(map[string]any)["item_properties"].(map[string]any)["minecraft:icon"].(map[string]any)["textures"]; !ok {
		t.Fatal("source data was mutated")
	}
}

// TestLegacyEntriesArcShapedForTheClientPin the alphabet rewrite the 1.16.x
// component packet applies: those clients read the fields by NBT tag type, and
// an item whose animation arrives as a number, duration in ticks or food
// without its hunger fields is marked unknown - an invisible stack the player
// cannot use. The modern registries keep their own representation.
func TestLegacyEntriesArcShapedForTheClient(t *testing.T) {
	data := map[string]any{"components": map[string]any{
		"item_properties": map[string]any{
			"minecraft:icon":    map[string]any{"textures": map[string]any{"default": "example:menu"}},
			"use_animation":     int32(2),
			"use_duration":      int32(32),
			"max_stack_size":    int32(16),
			"creative_category": int32(1),
		},
		"minecraft:food": map[string]any{"can_always_eat": true},
	}}
	custom := protocol.ItemEntry{Name: "example:menu", RuntimeID: 2, ComponentBased: true, Version: 1, Data: data}
	native := []protocol.ItemEntry{{Name: "minecraft:stone", RuntimeID: 1}, custom}
	target := map[string]TargetItem{"minecraft:stone": {RuntimeID: 2}}
	mapper, err := NewItemMapper(native, target)
	if err != nil {
		t.Fatal(err)
	}

	for _, version := range []int32{419, 475, 486} {
		out := mapper.RegistryEntries([]protocol.ItemEntry{custom}, int32(version))
		if len(out) != 1 {
			t.Fatalf("protocol %d lost its entries", version)
		}
		comps := out[0].Data["components"].(map[string]any)
		props := comps["item_properties"].(map[string]any)
		if icon, ok := props["minecraft:icon"].(map[string]any); !ok || icon["texture"] != "example:menu" {
			t.Fatalf("protocol %d did not flatten the icon: %v", version, props["minecraft:icon"])
		}
		if props["use_animation"] != "drink" {
			t.Fatalf("protocol %d kept a numeric drink animation: %v", version, props["use_animation"])
		}
		if d, ok := props["use_duration"].(float32); !ok || d != 1.6 {
			t.Fatalf("protocol %d kept a tick duration: %v", version, props["use_duration"])
		}
		food := comps["minecraft:food"].(map[string]any)
		if f, ok := food["nutrition"].(int32); !ok || f != 0 {
			t.Fatalf("protocol %d did not fill the food nutrition: %v", version, food["nutrition"])
		}
		if s, ok := food["saturation_modifier"].(float32); !ok || s != 0 {
			t.Fatalf("protocol %d did not fill the food saturation: %v", version, food["saturation_modifier"])
		}
	}

	out := mapper.RegistryEntries([]protocol.ItemEntry{custom}, 766)
	comps := out[0].Data["components"].(map[string]any)
	props := comps["item_properties"].(map[string]any)
	if props["use_animation"] != int32(2) || props["use_duration"] != int32(32) {
		t.Fatal("the modern registry lost its native field representation")
	}
	if _, ok := props["minecraft:icon"].(map[string]any)["textures"]; !ok {
		t.Fatal("the modern registry lost its icon shape")
	}
	food := comps["minecraft:food"].(map[string]any)
	if _, has := food["nutrition"]; has {
		t.Fatal("the modern registry should keep its own fields untouched")
	}
}
