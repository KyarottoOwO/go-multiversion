package compat

import (
	"bytes"
	"testing"

	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// TestEveryNativeBlockResolves checks the missing-block policy end to end: every
// native runtime ID must reach every supported version as exactly one block
// update, the conversion must be deterministic, and the target form must
// convert back to a native block that maps to the same target bytes again.
//
// A drop here means the server would silently send no update. Visual
// equivalence of the chosen fallback stays a version-data question; this test
// only proves that no native block ends up without a representable result.
func TestEveryNativeBlockResolves(t *testing.T) {
	blocks, _ := fixture(t)
	for _, p := range catalogue(t) {
		for runtimeID := uint32(0); runtimeID < uint32(len(blocks)); runtimeID++ {
			update := &packet.UpdateBlock{NewBlockRuntimeID: runtimeID}
			first := p.ConvertFromLatest(update, nil)
			if len(first) != 1 {
				t.Fatalf("protocol %d dropped native block %d: conversion returned %d packets", p.ID(), runtimeID, len(first))
			}
			firstWire := encodeWithAdapter(p, first[0])
			second := p.ConvertFromLatest(&packet.UpdateBlock{NewBlockRuntimeID: runtimeID}, nil)
			if len(second) != 1 {
				t.Fatalf("protocol %d dropped native block %d on the second conversion", p.ID(), runtimeID)
			}
			if secondWire := encodeWithAdapter(p, second[0]); !bytes.Equal(firstWire, secondWire) {
				t.Fatalf("protocol %d maps native block %d differently on each conversion", p.ID(), runtimeID)
			}
			back := p.ConvertToLatest(first[0], nil)
			if len(back) != 1 {
				t.Fatalf("protocol %d target block %d has no native form: conversion returned %d packets", p.ID(), runtimeID, len(back))
			}
			again := p.ConvertFromLatest(back[0], nil)
			if len(again) != 1 {
				t.Fatalf("protocol %d dropped native block %d after a round trip", p.ID(), runtimeID)
			}
			if againWire := encodeWithAdapter(p, again[0]); !bytes.Equal(firstWire, againWire) {
				t.Fatalf("protocol %d changes native block %d after a round trip", p.ID(), runtimeID)
			}
		}
	}
}
