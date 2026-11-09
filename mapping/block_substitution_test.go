package mapping

import "testing"

type substitutionTestRegistry struct{ states []BlockState }

func (r substitutionTestRegistry) BlockCount() int    { return len(r.states) }
func (substitutionTestRegistry) AirRuntimeID() uint32 { return 0 }

func (r substitutionTestRegistry) RuntimeIDToState(runtimeID uint32) (string, map[string]any, bool) {
	if runtimeID >= uint32(len(r.states)) {
		return "", nil, false
	}
	state := r.states[runtimeID]
	return state.Name, state.Properties, true
}

func TestBlockSubstitutions(t *testing.T) {
	native := substitutionTestRegistry{states: []BlockState{
		{Name: "minecraft:air"},
		{Name: "minecraft:birch_stairs", Properties: map[string]any{"weirdo_direction": int32(0), "upside_down_bit": false}},
		{Name: "minecraft:birch_stairs", Properties: map[string]any{"weirdo_direction": int32(1), "upside_down_bit": true}},
		{Name: "minecraft:element_1"},
		{Name: "minecraft:cherry_stairs", Properties: map[string]any{"minecraft:corner": "none", "weirdo_direction": int32(0), "upside_down_bit": false}},
		{Name: "minecraft:cherry_stairs", Properties: map[string]any{"minecraft:corner": "outer_left", "weirdo_direction": int32(1), "upside_down_bit": true}},
		{Name: "minecraft:cherry_planks"},
		{Name: "minecraft:deepslate"},
	}}
	historical := []BlockState{
		{Name: "minecraft:air"},
		{Name: "minecraft:birch_stairs", Properties: map[string]any{"weirdo_direction": int32(0), "upside_down_bit": uint8(0)}},
		{Name: "minecraft:birch_stairs", Properties: map[string]any{"weirdo_direction": int32(1), "upside_down_bit": uint8(1)}},
		{Name: "minecraft:element_1"},
	}
	substitutions := []BlockSubstitution{
		{Source: "minecraft:cherry_stairs", Target: "minecraft:birch_stairs", CopyProperties: []string{"weirdo_direction", "upside_down_bit"}},
		{Source: "minecraft:cherry_planks", Target: "minecraft:element_1"},
	}

	mapper, err := NewBlockMapperWithTargetOrderAndOptions(native, historical, BlockMapperOptions{Substitutions: substitutions})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := mapper.SubstitutedCount(), 3; got != want {
		t.Fatalf("substituted states = %d, want %d", got, want)
	}
	check := func(runtimeID, wantTarget uint32, wantName string) {
		t.Helper()
		target, valid, exact := mapper.MapNative(runtimeID)
		if !valid {
			t.Fatalf("native state %d is not valid", runtimeID)
		}
		if exact {
			t.Fatalf("native state %d is exact; substituted states must remain non-exact", runtimeID)
		}
		if target != wantTarget {
			t.Fatalf("native state %d maps to %d, want %d", runtimeID, target, wantTarget)
		}
		state, ok := mapper.TargetState(target)
		if !ok || state.Name != wantName {
			t.Fatalf("target state %d = %#v, want %s", target, state, wantName)
		}
	}
	check(4, 1, "minecraft:birch_stairs")
	check(5, 2, "minecraft:birch_stairs")
	check(6, 3, "minecraft:element_1")
	check(7, 0, "minecraft:air")

	stairs, ok := mapper.TargetState(2)
	if !ok {
		t.Fatal("birch target state is absent")
	}
	if stairs.Properties["weirdo_direction"] != int32(1) || stairs.Properties["upside_down_bit"] != uint8(1) {
		t.Fatalf("carried properties = %#v, want weirdo_direction=1 and upside_down_bit=1", stairs.Properties)
	}
	if _, exists := stairs.Properties["minecraft:corner"]; exists {
		t.Fatalf("non-copied property leaked into the target state: %#v", stairs.Properties)
	}

	fallbacks := make(map[uint32]BlockFallback)
	for _, fallback := range mapper.Fallbacks() {
		fallbacks[fallback.NativeRuntimeID] = fallback
	}
	for nativeID, wantTarget := range map[uint32]uint32{4: 1, 5: 2, 6: 3, 7: 0} {
		fallback, ok := fallbacks[nativeID]
		if !ok {
			t.Fatalf("native state %d has no fallback entry", nativeID)
		}
		if fallback.TargetRuntimeID != wantTarget {
			t.Fatalf("fallback %d target = %d, want %d", nativeID, fallback.TargetRuntimeID, wantTarget)
		}
	}

	plain, err := NewBlockMapperWithTargetOrder(native, historical)
	if err != nil {
		t.Fatal(err)
	}
	if plain.SubstitutedCount() != 0 {
		t.Fatalf("default mapper substituted %d states, want 0", plain.SubstitutedCount())
	}
	for nativeID := uint32(4); nativeID <= 7; nativeID++ {
		target, valid, exact := plain.MapNative(nativeID)
		if !valid || exact || target != 0 {
			t.Fatalf("default mapping for native state %d = %d/%v/%v, want air/false/true", nativeID, target, valid, exact)
		}
	}

	missing, err := NewBlockMapperWithTargetOrderAndOptions(native, historical, BlockMapperOptions{
		Substitutions: []BlockSubstitution{{Source: "minecraft:cherry_planks", Target: "minecraft:missing"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if target, _, exact := missing.MapNative(6); target != 0 || exact {
		t.Fatalf("missing target mapped to %d (exact=%v), want air", target, exact)
	}
	if missing.SubstitutedCount() != 0 {
		t.Fatalf("missing target substituted %d states, want 0", missing.SubstitutedCount())
	}

	incomplete, err := NewBlockMapperWithTargetOrderAndOptions(native, historical, BlockMapperOptions{
		Substitutions: []BlockSubstitution{{Source: "minecraft:cherry_planks", Target: "minecraft:element_1", CopyProperties: []string{"weirdo_direction"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if target, _, exact := incomplete.MapNative(6); target != 0 || exact {
		t.Fatalf("incomplete property carry mapped to %d (exact=%v), want air", target, exact)
	}
}
