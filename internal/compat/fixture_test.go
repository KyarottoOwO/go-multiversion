// Package compat holds cross-protocol harnesses. It is test-only so nothing in
// it can leak into the public API.
package compat

import (
	"maps"
	"sync"
	"testing"

	"github.com/df-mc/worldupgrader/blockupgrader"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	multiversion "github.com/shawtymarco/go-multiversion"
	native "github.com/shawtymarco/go-multiversion/data/v2225"
	"github.com/shawtymarco/go-multiversion/mapping"
)

type registry []mapping.BlockState

func (r registry) BlockCount() int { return len(r) }
func (r registry) AirRuntimeID() uint32 {
	for id, state := range r {
		if state.Name == "minecraft:air" {
			return uint32(id)
		}
	}
	panic("fixture has no air")
}
func (r registry) RuntimeIDToState(id uint32) (string, map[string]any, bool) {
	if id >= uint32(len(r)) {
		return "", nil, false
	}
	return r[id].Name, maps.Clone(r[id].Properties), true
}

var (
	fixtureOnce   sync.Once
	fixtureBlocks registry
	fixtureItems  []protocol.ItemEntry
	fixtureErr    error
)

func fixture(tb testing.TB) (registry, []protocol.ItemEntry) {
	tb.Helper()
	fixtureOnce.Do(func() {
		states, err := native.BlockStates()
		if err != nil {
			fixtureErr = err
			return
		}
		out := make(registry, 0, len(states))
		seen := make(map[string]bool, len(states))
		for i := len(states) - 1; i >= 0; i-- {
			state := states[i]
			upgraded := blockupgrader.Upgrade(blockupgrader.BlockState{Name: state.Name, Properties: maps.Clone(state.Properties), Version: state.Version})
			key, err := mapping.StateKey(upgraded.Name, upgraded.Properties)
			if err != nil {
				fixtureErr = err
				return
			}
			if !seen[key] {
				out = append(out, mapping.BlockState{Name: upgraded.Name, Properties: upgraded.Properties, Version: upgraded.Version})
				seen[key] = true
			}
		}
		snapshot, err := native.Items()
		if err != nil {
			fixtureErr = err
			return
		}
		items := make([]protocol.ItemEntry, 0, len(snapshot))
		for name, item := range snapshot {
			items = append(items, protocol.ItemEntry{Name: name, RuntimeID: int16(item.RuntimeID), ComponentBased: item.ComponentBased, Version: item.Version, Data: item.Data})
		}
		fixtureBlocks, fixtureItems = out, items
	})
	if fixtureErr != nil {
		tb.Fatal(fixtureErr)
	}
	return fixtureBlocks, fixtureItems
}

// catalogue returns every adapter the default registry-aware catalogue
// advertises, with the native model first.
func catalogue(tb testing.TB) []minecraft.Protocol {
	tb.Helper()
	blocks, items := fixture(tb)
	adapters, err := multiversion.ProtocolsWithRegistries(blocks, items)
	if err != nil {
		tb.Fatal(err)
	}
	return adapters
}
