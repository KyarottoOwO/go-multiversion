package mapping

import (
	"fmt"
	"hash/fnv"
	"sort"
	"strings"

	"github.com/df-mc/worldupgrader/blockupgrader"
)

// BlockRegistry is the native registry surface required to build a direct
// protocol mapping. Dragonfly's world.BlockRegistry satisfies this interface.
type BlockRegistry interface {
	BlockCount() int
	AirRuntimeID() uint32
	RuntimeIDToState(runtimeID uint32) (name string, properties map[string]any, found bool)
}

// BlockState is one historical runtime block state.
type BlockState struct {
	Name       string
	Properties map[string]any
	Version    int32
}

// BlockFallback records one native state that protocol 1001 cannot represent
// exactly. TargetRuntimeID is the historical air state unless an opt-in
// substitution resolved another target state.
type BlockFallback struct {
	NativeRuntimeID uint32
	Name            string
	Properties      map[string]any
	TargetRuntimeID uint32
}

// BlockSubstitution maps a native block name that the historical protocol
// cannot represent onto a target state. CopyProperties lists the native
// property names carried to the target; every other property is dropped.
// Substituted states remain non-exact so item and inventory conversion keeps
// rejecting them; only block rendering uses the substitute.
type BlockSubstitution struct {
	Source         string
	Target         string
	CopyProperties []string
}

// BlockMapperOptions configures opt-in mapping behaviour.
type BlockMapperOptions struct {
	// Substitutions are applied only to native states that have no exact target.
	// Every target is resolved in the loaded historical palette. A missing
	// target, or a missing CopyProperties value, leaves the air fallback.
	Substitutions []BlockSubstitution
}

// BlockMapper maps current native runtime IDs directly to historical target
// runtime IDs and back using identifier plus complete typed properties.
type BlockMapper struct {
	nativeToTarget []uint32
	nativeExact    []bool
	nativeStates   []BlockState
	targetToNative []uint32
	targetStates   []BlockState
	targetByKey    map[string]uint32
	fallbacks      []BlockFallback
	targetAir      uint32
	substituted    int
}

// NewBlockMapper builds an immutable direct mapping. Historical states are
// ordered using the stable name-only FNV-1 sort used by recent Dragonfly block
// registries before their runtime IDs are assigned.
func NewBlockMapper(native BlockRegistry, historical []BlockState) (*BlockMapper, error) {
	return newBlockMapper(native, historical, true, BlockMapperOptions{})
}

// NewBlockMapperWithTargetOrder builds an immutable direct mapping while
// preserving the input historical state order as the target runtime-ID order.
// It is used by releases whose Dragonfly registry registered the embedded NBT
// stream directly without a final sorting pass.
func NewBlockMapperWithTargetOrder(native BlockRegistry, historical []BlockState) (*BlockMapper, error) {
	return newBlockMapper(native, historical, false, BlockMapperOptions{})
}

// NewBlockMapperWithTargetOrderAndOptions builds an order-preserving mapping
// with opt-in substitutions for native states the target cannot represent.
func NewBlockMapperWithTargetOrderAndOptions(native BlockRegistry, historical []BlockState, options BlockMapperOptions) (*BlockMapper, error) {
	return newBlockMapper(native, historical, false, options)
}

func newBlockMapper(native BlockRegistry, historical []BlockState, sortTarget bool, options BlockMapperOptions) (*BlockMapper, error) {
	if native == nil {
		return nil, fmt.Errorf("native block registry is nil")
	}
	targetStates := append([]BlockState(nil), historical...)
	if sortTarget {
		sort.SliceStable(targetStates, func(i, j int) bool {
			return fnv1String(targetStates[i].Name) < fnv1String(targetStates[j].Name)
		})
	}

	targetByKey := make(map[string]uint32, len(targetStates))
	targetAir, foundAir := uint32(0), false
	for index, state := range targetStates {
		key, err := StateKey(state.Name, state.Properties)
		if err != nil {
			return nil, fmt.Errorf("target runtime ID %d: %w", index, err)
		}
		if _, exists := targetByKey[key]; exists {
			return nil, fmt.Errorf("duplicate target block state %s at runtime ID %d", state.Name, index)
		}
		targetByKey[key] = uint32(index)
		if state.Name == "minecraft:air" {
			targetAir, foundAir = uint32(index), true
		}
	}
	if !foundAir {
		return nil, fmt.Errorf("target block registry has no minecraft:air state")
	}

	nativeByKey := make(map[string]uint32, native.BlockCount())
	nativeAliases := make(map[string]uint32)
	nativeStates := make([]BlockState, native.BlockCount())
	for index := range native.BlockCount() {
		runtimeID := uint32(index)
		name, properties, ok := native.RuntimeIDToState(runtimeID)
		if !ok {
			return nil, fmt.Errorf("native runtime ID %d has no state", runtimeID)
		}
		key, err := StateKey(name, properties)
		if err != nil {
			return nil, fmt.Errorf("native runtime ID %d: %w", runtimeID, err)
		}
		if _, exists := nativeByKey[key]; exists {
			return nil, fmt.Errorf("duplicate native block state %s at runtime ID %d", name, runtimeID)
		}
		nativeByKey[key] = runtimeID
		aliasKey, err := StateKey(name, nativeCompatibilityProperties(name, properties))
		if err != nil {
			return nil, fmt.Errorf("native runtime ID %d compatibility alias: %w", runtimeID, err)
		}
		if aliasKey != key {
			if existing, exists := nativeAliases[aliasKey]; exists && existing != runtimeID {
				return nil, fmt.Errorf("duplicate native compatibility state %s at runtime IDs %d and %d", name, existing, runtimeID)
			}
			nativeAliases[aliasKey] = runtimeID
		}
		nativeStates[index] = BlockState{Name: name, Properties: cloneProperties(properties)}
	}

	targetToNative := make([]uint32, len(targetStates))
	targetCompatibleByNativeKey := make(map[string]uint32, len(targetStates))
	unresolved := make([]string, 0)
	for targetRuntimeID, state := range targetStates {
		key, _ := StateKey(state.Name, state.Properties)
		nativeRuntimeID, ok := nativeByKey[key]
		if !ok {
			// New native releases may move client-derived visual state into the
			// server registry. Older targets omit these properties entirely, so
			// resolve them to the neutral native state while preserving every
			// property the target did carry.
			for _, properties := range nativeCompatibilityVariants(state.Properties) {
				compatibleKey, err := StateKey(state.Name, properties)
				if err != nil {
					return nil, fmt.Errorf("compatible target runtime ID %d: %w", targetRuntimeID, err)
				}
				if nativeRuntimeID, ok = nativeByKey[compatibleKey]; ok {
					break
				}
			}
		}
		if !ok {
			upgraded := blockupgrader.Upgrade(blockupgrader.BlockState{
				Name:       state.Name,
				Properties: cloneProperties(state.Properties),
				Version:    state.Version,
			})
			upgradedKey, err := StateKey(upgraded.Name, upgraded.Properties)
			if err != nil {
				return nil, fmt.Errorf("upgraded target runtime ID %d: %w", targetRuntimeID, err)
			}
			nativeRuntimeID, ok = nativeByKey[upgradedKey]
			if !ok {
				nativeRuntimeID, ok = nativeAliases[upgradedKey]
			}
			if !ok {
				for _, properties := range nativeCompatibilityVariants(upgraded.Properties) {
					compatibleKey, err := StateKey(upgraded.Name, properties)
					if err != nil {
						return nil, fmt.Errorf("compatible upgraded target runtime ID %d: %w", targetRuntimeID, err)
					}
					if nativeRuntimeID, ok = nativeByKey[compatibleKey]; ok {
						break
					}
				}
			}
			if !ok {
				if len(unresolved) < 32 {
					unresolved = append(unresolved, fmt.Sprintf("%s%v (RID %d) -> %s%v", state.Name, state.Properties, targetRuntimeID, upgraded.Name, upgraded.Properties))
				}
				continue
			}
		}
		targetToNative[targetRuntimeID] = nativeRuntimeID
		resolved := nativeStates[nativeRuntimeID]
		compatibleKey, err := StateKey(resolved.Name, stripNativeCompatibilityProperties(resolved.Properties))
		if err != nil {
			return nil, fmt.Errorf("resolved target runtime ID %d: %w", targetRuntimeID, err)
		}
		if _, exists := targetCompatibleByNativeKey[compatibleKey]; !exists {
			targetCompatibleByNativeKey[compatibleKey] = uint32(targetRuntimeID)
		}
	}
	if len(unresolved) != 0 {
		return nil, fmt.Errorf("target registry has unresolved native states:\n%s", strings.Join(unresolved, "\n"))
	}

	nativeToTarget := make([]uint32, native.BlockCount())
	nativeExact := make([]bool, native.BlockCount())
	for runtimeID := range nativeToTarget {
		nativeToTarget[runtimeID] = targetAir
		state := nativeStates[runtimeID]
		key, _ := StateKey(state.Name, state.Properties)
		if targetRuntimeID, ok := targetByKey[key]; ok {
			nativeToTarget[runtimeID], nativeExact[runtimeID] = targetRuntimeID, true
			continue
		}
		aliasKey, _ := StateKey(state.Name, nativeCompatibilityProperties(state.Name, state.Properties))
		if targetRuntimeID, ok := targetByKey[aliasKey]; ok {
			nativeToTarget[runtimeID], nativeExact[runtimeID] = targetRuntimeID, true
			continue
		}
		stripped := stripNativeCompatibilityProperties(state.Properties)
		if len(stripped) != len(state.Properties) {
			compatibleKey, _ := StateKey(state.Name, stripped)
			if targetRuntimeID, ok := targetByKey[compatibleKey]; ok {
				nativeToTarget[runtimeID], nativeExact[runtimeID] = targetRuntimeID, true
				continue
			}
			if targetRuntimeID, ok := targetCompatibleByNativeKey[compatibleKey]; ok {
				nativeToTarget[runtimeID], nativeExact[runtimeID] = targetRuntimeID, true
			}
		}
	}
	for targetRuntimeID, nativeRuntimeID := range targetToNative {
		if !nativeExact[nativeRuntimeID] {
			nativeToTarget[nativeRuntimeID], nativeExact[nativeRuntimeID] = uint32(targetRuntimeID), true
		}
	}

	// Opt-in substitutions still count as non-exact. They only change the
	// runtime ID used for block rendering; item conversion continues to see the
	// state as unrepresentable.
	substituted := make(map[uint32]uint32, len(options.Substitutions))
	for runtimeID := range nativeExact {
		if nativeExact[runtimeID] {
			continue
		}
		target, ok := substituteTarget(nativeStates[runtimeID], options.Substitutions, targetByKey)
		if !ok {
			continue
		}
		nativeToTarget[runtimeID] = target
		substituted[uint32(runtimeID)] = target
	}

	fallbacks := make([]BlockFallback, 0)
	for runtimeID, exact := range nativeExact {
		if exact {
			continue
		}
		state := nativeStates[runtimeID]
		target := targetAir
		if substitute, ok := substituted[uint32(runtimeID)]; ok {
			target = substitute
		}
		fallbacks = append(fallbacks, BlockFallback{
			NativeRuntimeID: uint32(runtimeID),
			Name:            state.Name,
			Properties:      cloneProperties(state.Properties),
			TargetRuntimeID: target,
		})
	}

	return &BlockMapper{
		nativeToTarget: nativeToTarget,
		nativeExact:    nativeExact,
		nativeStates:   nativeStates,
		targetToNative: targetToNative,
		targetStates:   targetStates,
		targetByKey:    targetByKey,
		fallbacks:      fallbacks,
		targetAir:      targetAir,
		substituted:    len(substituted),
	}, nil
}

// NativeState returns the current block state represented by runtimeID.
func (m *BlockMapper) NativeState(runtimeID uint32) (BlockState, bool) {
	if m == nil || runtimeID >= uint32(len(m.nativeStates)) {
		return BlockState{}, false
	}
	state := m.nativeStates[runtimeID]
	state.Properties = cloneProperties(state.Properties)
	return state, true
}

// TargetState returns the historical block state represented by runtimeID.
func (m *BlockMapper) TargetState(runtimeID uint32) (BlockState, bool) {
	if m == nil || runtimeID >= uint32(len(m.targetStates)) {
		return BlockState{}, false
	}
	state := m.targetStates[runtimeID]
	state.Properties = cloneProperties(state.Properties)
	return state, true
}

// NativeToTarget maps a current runtime ID to protocol 1001. exact is false
// when the explicit minecraft:air fallback was used.
func (m *BlockMapper) NativeToTarget(runtimeID uint32) (target uint32, exact bool) {
	if m == nil || runtimeID >= uint32(len(m.nativeToTarget)) {
		if m == nil {
			return 0, false
		}
		return m.targetAir, false
	}
	return m.nativeToTarget[runtimeID], m.nativeExact[runtimeID]
}

// MapNative maps a current runtime ID and distinguishes an explicit fallback
// from an out-of-range ID.
func (m *BlockMapper) MapNative(runtimeID uint32) (target uint32, valid bool, exact bool) {
	if m == nil || runtimeID >= uint32(len(m.nativeToTarget)) {
		return 0, false, false
	}
	return m.nativeToTarget[runtimeID], true, m.nativeExact[runtimeID]
}

// TargetToNative maps a protocol-1001 runtime ID to the exact current state.
func (m *BlockMapper) TargetToNative(runtimeID uint32) (uint32, bool) {
	if m == nil || runtimeID >= uint32(len(m.targetToNative)) {
		return 0, false
	}
	return m.targetToNative[runtimeID], true
}

// TargetRuntimeID resolves a historical state by identifier and properties.
func (m *BlockMapper) TargetRuntimeID(name string, properties map[string]any) (uint32, bool) {
	if m == nil {
		return 0, false
	}
	key, err := StateKey(name, properties)
	if err != nil {
		return 0, false
	}
	runtimeID, ok := m.targetByKey[key]
	return runtimeID, ok
}

// TargetStates returns the historical states in runtime-ID order.
func (m *BlockMapper) TargetStates() []BlockState {
	if m == nil {
		return nil
	}
	states := make([]BlockState, len(m.targetStates))
	for index, state := range m.targetStates {
		states[index] = BlockState{Name: state.Name, Properties: cloneProperties(state.Properties), Version: state.Version}
	}
	return states
}

// Fallbacks returns every native state without an exact target. Entries use
// historical air unless an opt-in substitution resolved a target state.
func (m *BlockMapper) Fallbacks() []BlockFallback {
	if m == nil {
		return nil
	}
	fallbacks := make([]BlockFallback, len(m.fallbacks))
	for index, fallback := range m.fallbacks {
		fallbacks[index] = fallback
		fallbacks[index].Properties = cloneProperties(fallback.Properties)
	}
	return fallbacks
}

// SubstitutedCount returns the number of native states mapped through an
// opt-in substitution.
func (m *BlockMapper) SubstitutedCount() int {
	if m == nil {
		return 0
	}
	return m.substituted
}

// TargetAir returns the protocol-1001 air runtime ID.
func (m *BlockMapper) TargetAir() uint32 {
	if m == nil {
		return 0
	}
	return m.targetAir
}

// NativeCount returns the current registry size used to build the mapping.
func (m *BlockMapper) NativeCount() int {
	if m == nil {
		return 0
	}
	return len(m.nativeToTarget)
}

// TargetCount returns the historical registry size.
func (m *BlockMapper) TargetCount() int {
	if m == nil {
		return 0
	}
	return len(m.targetToNative)
}

// substituteTarget resolves the historical target state for one native state.
// Targets are never synthesised: the complete copied state must exist in the
// loaded palette.
func substituteTarget(state BlockState, substitutions []BlockSubstitution, targetByKey map[string]uint32) (uint32, bool) {
	for _, substitution := range substitutions {
		if substitution.Source != state.Name || substitution.Target == "" {
			continue
		}
		properties := make(map[string]any, len(substitution.CopyProperties))
		complete := true
		for _, name := range substitution.CopyProperties {
			value, ok := state.Properties[name]
			if !ok {
				complete = false
				break
			}
			properties[name] = value
		}
		if !complete {
			continue
		}
		key, err := StateKey(substitution.Target, properties)
		if err != nil {
			continue
		}
		if target, ok := targetByKey[key]; ok {
			return target, true
		}
	}
	return 0, false
}

func cloneProperties(properties map[string]any) map[string]any {
	if properties == nil {
		return nil
	}
	cloned := make(map[string]any, len(properties))
	for key, value := range properties {
		cloned[key] = value
	}
	return cloned
}

// nativeCompatibilityProperties normalises known third-party Dragonfly block
// implementations that still expose an old property name while representing a
// current vanilla state. bedrock-gophers/inv registers its dropper container
// with toggle_bit, whereas current and historical registries use
// triggered_bit. Treating the names as semantic aliases keeps the consumer's
// synthetic implementation mappable without weakening unresolved target-state
// validation for unrelated blocks.
func nativeCompatibilityProperties(name string, properties map[string]any) map[string]any {
	cloned := cloneProperties(properties)
	if name != "minecraft:dropper" && name != "minecraft:dispenser" {
		return cloned
	}
	if _, exists := cloned["triggered_bit"]; exists {
		return cloned
	}
	if value, exists := cloned["toggle_bit"]; exists {
		delete(cloned, "toggle_bit")
		cloned["triggered_bit"] = value
	}
	return cloned
}

var nativeCompatibilityDefaults = map[string]any{
	"minecraft:corner":           "none",
	"minecraft:connection_north": false,
	"minecraft:connection_east":  false,
	"minecraft:connection_south": false,
	"minecraft:connection_west":  false,
}

func nativeCompatibilityVariants(properties map[string]any) []map[string]any {
	variants := make([]map[string]any, 0, 2)
	if _, exists := properties["minecraft:corner"]; !exists {
		withCorner := cloneProperties(properties)
		if withCorner == nil {
			withCorner = map[string]any{}
		}
		withCorner["minecraft:corner"] = "none"
		variants = append(variants, withCorner)
	}
	missingConnection := false
	for name := range nativeCompatibilityDefaults {
		if name == "minecraft:corner" {
			continue
		}
		if _, exists := properties[name]; !exists {
			missingConnection = true
		}
	}
	if missingConnection {
		withConnections := cloneProperties(properties)
		if withConnections == nil {
			withConnections = map[string]any{}
		}
		for name, value := range nativeCompatibilityDefaults {
			if name != "minecraft:corner" {
				withConnections[name] = value
			}
		}
		variants = append(variants, withConnections)
		// Native 26.60 fire adds an upper flame face as well. Keep the
		// four-face candidate for panes/fences and try the five-face state
		// separately; adding an upper face to every block would be invalid.
		withUpper := cloneProperties(withConnections)
		withUpper["minecraft:connection_up"] = false
		variants = append(variants, withUpper)
		withVertical := cloneProperties(withUpper)
		withVertical["minecraft:connection_down"] = false
		variants = append(variants, withVertical)
	}
	if _, wire := properties["redstone_signal"]; wire {
		withWire := cloneProperties(properties)
		for _, key := range []string{"redstone_north", "redstone_east", "redstone_south", "redstone_west"} {
			if _, exists := withWire[key]; !exists {
				withWire[key] = "none"
			}
		}
		variants = append(variants, withWire)
	}
	return variants
}

func stripNativeCompatibilityProperties(properties map[string]any) map[string]any {
	stripped := cloneProperties(properties)
	for name := range nativeCompatibilityDefaults {
		delete(stripped, name)
	}
	delete(stripped, "minecraft:connection_up")
	delete(stripped, "minecraft:connection_down")
	for _, key := range []string{"redstone_north", "redstone_east", "redstone_south", "redstone_west"} {
		delete(stripped, key)
	}
	return stripped
}

func fnv1String(value string) uint64 {
	hash := fnv.New64()
	_, _ = hash.Write([]byte(value))
	return hash.Sum64()
}
