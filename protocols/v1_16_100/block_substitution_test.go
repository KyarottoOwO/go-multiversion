package v1_16_100

import (
	"testing"

	"github.com/df-mc/worldupgrader/blockupgrader"
	v419data "github.com/shawtymarco/go-multiversion/data/v419"
	"github.com/shawtymarco/go-multiversion/mapping"
)

// TestProtocol419SubstitutionPalette confirms the three substitution targets
// against the embedded 1.16.100 palette instead of assuming their names.
func TestProtocol419SubstitutionPalette(t *testing.T) {
	historical, err := v419data.BlockStates()
	if err != nil {
		t.Fatal(err)
	}
	elementTarget, ok := protocol419TargetRuntimeID(historical, "minecraft:element_1", map[string]any{})
	if !ok {
		t.Fatal("protocol 419 palette does not contain minecraft:element_1")
	}
	t.Logf("minecraft:element_1 target runtime ID = %d", elementTarget)

	birchStates := 0
	for _, state := range historical {
		if state.Name != "minecraft:birch_stairs" {
			continue
		}
		birchStates++
		if len(state.Properties) != 2 {
			t.Fatalf("birch_stairs properties = %#v, want weirdo_direction and upside_down_bit", state.Properties)
		}
		if _, ok := state.Properties["weirdo_direction"]; !ok {
			t.Fatalf("birch_stairs properties = %#v, want weirdo_direction", state.Properties)
		}
		if _, ok := state.Properties["upside_down_bit"]; !ok {
			t.Fatalf("birch_stairs properties = %#v, want upside_down_bit", state.Properties)
		}
	}
	if birchStates != 8 {
		t.Fatalf("birch_stairs states = %d, want 8", birchStates)
	}
	for _, name := range []string{"minecraft:cherry_stairs", "minecraft:cherry_planks", "minecraft:deepslate"} {
		if _, ok := protocol419TargetRuntimeID(historical, name, map[string]any{}); ok {
			t.Fatalf("%s unexpectedly exists in the 1.16.100 palette", name)
		}
	}
}

func TestProtocol419BlockSubstitutions(t *testing.T) {
	registry, historical := protocol419SubstitutionRegistries(t)
	airTarget, ok := protocol419TargetRuntimeID(historical, "minecraft:air", map[string]any{})
	if !ok {
		t.Fatal("protocol 419 palette does not contain minecraft:air")
	}
	birchTarget, ok := protocol419TargetRuntimeID(historical, "minecraft:birch_stairs", map[string]any{
		"weirdo_direction": int32(3),
		"upside_down_bit":  true,
	})
	if !ok {
		t.Fatal("protocol 419 palette does not contain the target birch_stairs state")
	}
	elementTarget, ok := protocol419TargetRuntimeID(historical, "minecraft:element_1", map[string]any{})
	if !ok {
		t.Fatal("protocol 419 palette does not contain minecraft:element_1")
	}

	cherryStairs := protocol419NativeRuntimeID(t, registry, "minecraft:cherry_stairs", map[string]any{
		"minecraft:corner": "none",
		"weirdo_direction": int32(3),
		"upside_down_bit":  true,
	})
	cherryPlanks := protocol419NativeRuntimeID(t, registry, "minecraft:cherry_planks", map[string]any{})
	deepslate := protocol419NativeRuntimeID(t, registry, "minecraft:deepslate", map[string]any{})

	substitutions := []mapping.BlockSubstitution{
		{Source: "minecraft:cherry_stairs", Target: "minecraft:birch_stairs", CopyProperties: []string{"weirdo_direction", "upside_down_bit"}},
		{Source: "minecraft:cherry_planks", Target: "minecraft:element_1"},
	}
	adapter, err := NewWithRegistriesAndOptions(registry, nil, MappingOptions{BlockSubstitutions: substitutions})
	if err != nil {
		t.Fatal(err)
	}
	protocol, ok := adapter.(*Protocol)
	if !ok {
		t.Fatalf("adapter type = %T, want *Protocol", adapter)
	}

	report := protocol.MappingReport()
	if got, want := report.SubstitutedFallbacks, 2; got != want {
		t.Fatalf("substituted fallbacks = %d, want %d", got, want)
	}
	for nativeID, want := range map[uint32]uint32{
		cherryStairs: birchTarget,
		cherryPlanks: elementTarget,
		deepslate:    airTarget,
	} {
		target, valid := protocol.MapBlockRuntimeID(nativeID)
		if !valid {
			t.Fatalf("native state %d is not valid", nativeID)
		}
		if target != want {
			t.Fatalf("native state %d maps to %d, want %d", nativeID, target, want)
		}
	}

	fallbacks := make(map[uint32]mapping.BlockFallback, len(report.BlockFallbacks))
	for _, fallback := range report.BlockFallbacks {
		fallbacks[fallback.NativeRuntimeID] = fallback
	}
	for _, nativeID := range []uint32{cherryStairs, cherryPlanks, deepslate} {
		if _, ok := fallbacks[nativeID]; !ok {
			t.Fatalf("native state %d is not reported as a non-exact fallback", nativeID)
		}
	}

	plainAdapter, err := NewWithRegistries(registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	plain, ok := plainAdapter.(*Protocol)
	if !ok {
		t.Fatalf("plain adapter type = %T, want *Protocol", plainAdapter)
	}
	if got := plain.MappingReport().SubstitutedFallbacks; got != 0 {
		t.Fatalf("default adapter substituted %d states, want 0", got)
	}
	for _, nativeID := range []uint32{cherryStairs, cherryPlanks} {
		if target, _ := plain.MapBlockRuntimeID(nativeID); target != airTarget {
			t.Fatalf("default mapping for native state %d = %d, want air %d", nativeID, target, airTarget)
		}
	}
}

func protocol419SubstitutionRegistries(t *testing.T) (blockItemTestRegistry, []v419data.BlockState) {
	t.Helper()
	historical, err := v419data.BlockStates()
	if err != nil {
		t.Fatal(err)
	}
	states := make([]mapping.BlockState, 0, len(historical)+3)
	seen := make(map[string]struct{}, len(historical)+3)
	add := func(name string, properties map[string]any, version int32) {
		key, err := mapping.StateKey(name, properties)
		if err != nil {
			t.Fatalf("state key for %s: %v", name, err)
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		states = append(states, mapping.BlockState{Name: name, Properties: cloneBlockItemTestProperties(properties), Version: version})
	}
	for _, state := range historical {
		upgraded := blockupgrader.Upgrade(blockupgrader.BlockState{
			Name:       state.Name,
			Properties: cloneBlockItemTestProperties(state.Properties),
			Version:    state.Version,
		})
		add(upgraded.Name, upgraded.Properties, upgraded.Version)
	}
	add("minecraft:cherry_stairs", map[string]any{"minecraft:corner": "none", "weirdo_direction": int32(3), "upside_down_bit": true}, 0)
	add("minecraft:cherry_planks", map[string]any{}, 0)
	add("minecraft:deepslate", map[string]any{}, 0)
	return blockItemTestRegistry{states: states}, historical
}

func protocol419TargetRuntimeID(states []v419data.BlockState, name string, properties map[string]any) (uint32, bool) {
	key, err := mapping.StateKey(name, properties)
	if err != nil {
		return 0, false
	}
	for runtimeID, state := range states {
		stateKey, err := mapping.StateKey(state.Name, state.Properties)
		if err != nil {
			continue
		}
		if stateKey == key {
			return uint32(runtimeID), true
		}
	}
	return 0, false
}

func protocol419NativeRuntimeID(t *testing.T, registry blockItemTestRegistry, name string, properties map[string]any) uint32 {
	t.Helper()
	key, err := mapping.StateKey(name, properties)
	if err != nil {
		t.Fatal(err)
	}
	for runtimeID, state := range registry.states {
		stateKey, err := mapping.StateKey(state.Name, state.Properties)
		if err != nil {
			t.Fatal(err)
		}
		if stateKey == key {
			return uint32(runtimeID)
		}
	}
	t.Fatalf("native state %s%v is not registered", name, properties)
	return 0
}
