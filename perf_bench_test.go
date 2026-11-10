package multiversion_test

import (
	"maps"
	"sync"
	"testing"

	"github.com/df-mc/worldupgrader/blockupgrader"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	multiversion "github.com/shawtymarco/go-multiversion"
	v2193 "github.com/shawtymarco/go-multiversion/data/v2193"
	native "github.com/shawtymarco/go-multiversion/data/v2225"
	v419 "github.com/shawtymarco/go-multiversion/data/v419"
	"github.com/shawtymarco/go-multiversion/mapping"
)

// The benchmarks below measure the two costs users actually pay: adapter
// catalogue construction at server start, and StartGame conversion per join.
// The registry fixture mirrors effectsProtocols so the mapping workload is the
// real 1.26.60 state set rather than a toy palette.

var (
	perfOnce     sync.Once
	perfRegistry effectsRegistry
	perfItems    []protocol.ItemEntry
	perfErr      error
)

func perfFixture(tb testing.TB) (effectsRegistry, []protocol.ItemEntry) {
	tb.Helper()
	perfOnce.Do(func() { perfRegistry, perfItems, perfErr = buildPerfFixture() })
	if perfErr != nil {
		tb.Fatal(perfErr)
	}
	return perfRegistry, perfItems
}

func buildPerfFixture() (effectsRegistry, []protocol.ItemEntry, error) {
	states, err := native.BlockStates()
	if err != nil {
		return nil, nil, err
	}
	registry := make(effectsRegistry, 0, len(states))
	seen := make(map[string]bool, len(states))
	for i := len(states) - 1; i >= 0; i-- {
		state := states[i]
		upgraded := blockupgrader.Upgrade(blockupgrader.BlockState{Name: state.Name, Properties: maps.Clone(state.Properties), Version: state.Version})
		key, err := mapping.StateKey(upgraded.Name, upgraded.Properties)
		if err != nil {
			return nil, nil, err
		}
		if !seen[key] {
			registry = append(registry, mapping.BlockState{Name: upgraded.Name, Properties: upgraded.Properties, Version: upgraded.Version})
			seen[key] = true
		}
	}
	for i := range registry {
		if registry[i].Name == "minecraft:lime_wool" {
			registry[0], registry[i] = registry[i], registry[0]
			break
		}
	}
	if len(registry) == 0 || registry[0].Name != "minecraft:lime_wool" {
		return nil, nil, errNoLimeWool
	}
	snapshot, err := native.Items()
	if err != nil {
		return nil, nil, err
	}
	items := make([]protocol.ItemEntry, 0, len(snapshot))
	for name, item := range snapshot {
		items = append(items, protocol.ItemEntry{Name: name, RuntimeID: int16(item.RuntimeID), ComponentBased: item.ComponentBased, Version: item.Version, Data: item.Data})
	}
	return registry, items, nil
}

type perfError string

func (e perfError) Error() string { return string(e) }

const errNoLimeWool = perfError("benchmark fixture has no minecraft:lime_wool state")

// BenchmarkProtocolsWithRegistries measures the documented startup entry point
// for the full default catalogue.
func BenchmarkProtocolsWithRegistries(b *testing.B) {
	registry, items := perfFixture(b)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := multiversion.ProtocolsWithRegistries(registry, items); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkProtocolsWithRegistriesFloor748 measures the minimum protocol the
// upstream Dragonfly integration accepts, which lets a floor-aware catalogue
// skip the Login-first adapters entirely.
func BenchmarkProtocolsWithRegistriesFloor748(b *testing.B) {
	registry, items := perfFixture(b)
	config := multiversion.Config{MinimumProtocol: 748}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := config.ProtocolsWithRegistries(registry, items); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkProtocolsWithRegistriesFloor2193 measures a native-adjacent floor,
// where only the outgoing 1.26.50 adapter can be advertised.
func BenchmarkProtocolsWithRegistriesFloor2193(b *testing.B) {
	registry, items := perfFixture(b)
	config := multiversion.Config{MinimumProtocol: 2193}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := config.ProtocolsWithRegistries(registry, items); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkProtocolsWithBlockRegistry measures the registry-only catalogue.
func BenchmarkProtocolsWithBlockRegistry(b *testing.B) {
	registry, _ := perfFixture(b)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := multiversion.ProtocolsWithBlockRegistry(registry); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkBlockDefinitions2193 isolates the embedded 80 KB NBT decode that
// StartGame conversion used to repeat for every joining player.
func BenchmarkBlockDefinitions2193(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := v2193.BlockDefinitions(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStartGameConvertFromLatest2193 measures the per-join StartGame
// downgrade for protocol 2193.
func BenchmarkStartGameConvertFromLatest2193(b *testing.B) {
	registry, items := perfFixture(b)
	proto, err := multiversion.V1_26_50WithRegistries(registry, items)
	if err != nil {
		b.Fatal(err)
	}
	blocks, err := v2193.BlockDefinitions()
	if err != nil {
		b.Fatal(err)
	}
	input := &packet.StartGame{
		EntityRuntimeID: 1,
		GameVersion:     "1.26.60",
		BaseGameVersion: "1.26.60",
		PlayerPosition:  [3]float32{0, 64, 0},
		WorldName:       "bench",
		Blocks:          blocks,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if out := proto.ConvertFromLatest(input, nil); len(out) == 0 {
			b.Fatal("StartGame conversion produced no packets")
		}
	}
}

// BenchmarkPackets2193 measures one packet-pool construction, which
// gophertunnel performs once per connection.
func BenchmarkPackets2193(b *testing.B) {
	adapter := multiversion.V1_26_50()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if pool := adapter.Packets(false); len(pool) == 0 {
			b.Fatal("empty packet pool")
		}
	}
}

func benchmarkMapperStates(b *testing.B) []mapping.BlockState {
	b.Helper()
	states, err := v2193.BlockStates()
	if err != nil {
		b.Fatal(err)
	}
	out := make([]mapping.BlockState, len(states))
	for i, state := range states {
		out[i] = mapping.BlockState{Name: state.Name, Properties: state.Properties, Version: state.Version}
	}
	return out
}

// BenchmarkNewBlockMapper measures the sorted (recent Dragonfly) mapping path.
func BenchmarkNewBlockMapper(b *testing.B) {
	registry, _ := perfFixture(b)
	states := benchmarkMapperStates(b)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := mapping.NewBlockMapper(registry, states); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkNewBlockMapper419 measures the order-preserving 1.16.100 path.
func BenchmarkNewBlockMapper419(b *testing.B) {
	registry, _ := perfFixture(b)
	states, err := v419.BlockStates()
	if err != nil {
		b.Fatal(err)
	}
	historical := make([]mapping.BlockState, len(states))
	for i, state := range states {
		historical[i] = mapping.BlockState{Name: state.Name, Properties: state.Properties, Version: state.Version}
	}
	options := mapping.BlockMapperOptions{Substitutions: []mapping.BlockSubstitution{{Source: "minecraft:unknown_to_419", Target: "minecraft:stone", CopyProperties: []string{"minecraft:pillar_axis"}}}}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := mapping.NewBlockMapperWithTargetOrderAndOptions(registry, historical, options); err != nil {
			b.Fatal(err)
		}
	}
}
