package mapping

import (
	"sync"
	"testing"
)

// TestReportFallbackOncePerKey pins the rate limit: repeated hits for one
// (protocol, kind, name) report once, and other keys still report.
func TestReportFallbackOncePerKey(t *testing.T) {
	ResetFallbackReports()
	var mu sync.Mutex
	var seen []Fallback
	previous := SetFallbackLogger(func(f Fallback) {
		mu.Lock()
		seen = append(seen, f)
		mu.Unlock()
	})
	defer SetFallbackLogger(previous)

	for range 5 {
		ReportFallback(Fallback{Protocol: 419, Kind: "block", Name: "minecraft:new_block", Used: "minecraft:air"})
	}
	ReportFallback(Fallback{Protocol: 419, Kind: "item", Name: "minecraft:new_block", Used: "dropped"})
	ReportFallback(Fallback{Protocol: 475, Kind: "block", Name: "minecraft:new_block", Used: "minecraft:air"})

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 3 {
		t.Fatalf("reports: got %d (%#v), want 3", len(seen), seen)
	}
	if seen[0].Kind != "block" || seen[1].Kind != "item" || seen[2].Protocol != 475 {
		t.Fatalf("report order: %#v", seen)
	}
}

// TestBlockMapperReportsFallbackOnce proves the mapper reports an inexact
// mapping itself, so every adapter call site that discards the exactness flag
// is still visible.
func TestBlockMapperReportsFallbackOnce(t *testing.T) {
	ResetFallbackReports()
	var mu sync.Mutex
	var seen []Fallback
	previous := SetFallbackLogger(func(f Fallback) {
		mu.Lock()
		seen = append(seen, f)
		mu.Unlock()
	})
	defer SetFallbackLogger(previous)

	mapper, err := NewBlockMapper(fallbackTestRegistry{}, []BlockState{{Name: "minecraft:air"}}, BlockMapperOptions{Protocol: 486})
	if err != nil {
		t.Fatal(err)
	}
	for range 4 {
		if target, exact := mapper.NativeToTarget(1); target != 0 || exact {
			t.Fatalf("fallback mapping: got %d/%v, want 0/false", target, exact)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 {
		t.Fatalf("reports: got %d (%#v), want 1", len(seen), seen)
	}
	if seen[0].Protocol != 486 || seen[0].Kind != "block" || seen[0].Name != "minecraft:new_block" || seen[0].Used != "minecraft:air" {
		t.Fatalf("report: %#v", seen[0])
	}
}

// fallbackTestRegistry holds air and one block the target palette lacks.
type fallbackTestRegistry struct{}

func (fallbackTestRegistry) BlockCount() int { return 2 }

func (fallbackTestRegistry) AirRuntimeID() uint32 { return 0 }

func (fallbackTestRegistry) RuntimeIDToState(runtimeID uint32) (string, map[string]any, bool) {
	switch runtimeID {
	case 0:
		return "minecraft:air", nil, true
	case 1:
		return "minecraft:new_block", nil, true
	}
	return "", nil, false
}
