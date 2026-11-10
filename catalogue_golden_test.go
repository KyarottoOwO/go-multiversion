package multiversion_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	multiversion "github.com/shawtymarco/go-multiversion"
)

// catalogueSnapshot locks the advertised catalogue: protocol IDs and their
// order for every public constructor and every documented MinimumProtocol
// floor. The golden file is selected by build tag, so the default and
// legacytransport catalogues are recorded independently.
type catalogueSnapshot struct {
	ProtocolsWithRegistries    []int32            `json:"protocols_with_registries"`
	ProtocolsWithBlockRegistry []int32            `json:"protocols_with_block_registry"`
	Config                     map[string][]int32 `json:"config"`
}

func protocolIDs(protocols []minecraft.Protocol) []int32 {
	ids := make([]int32, len(protocols))
	for i, p := range protocols {
		ids[i] = p.ID()
	}
	return ids
}

func catalogueSnapshotOf(t *testing.T) catalogueSnapshot {
	t.Helper()
	registry, items := perfFixture(t)
	registries, err := multiversion.ProtocolsWithRegistries(registry, items)
	if err != nil {
		t.Fatalf("ProtocolsWithRegistries: %v", err)
	}
	blockRegistry, err := multiversion.ProtocolsWithBlockRegistry(registry)
	if err != nil {
		t.Fatalf("ProtocolsWithBlockRegistry: %v", err)
	}
	snapshot := catalogueSnapshot{
		ProtocolsWithRegistries:    protocolIDs(registries),
		ProtocolsWithBlockRegistry: protocolIDs(blockRegistry),
		Config:                     make(map[string][]int32),
	}
	for _, floor := range []int32{0, 748, 2193, protocol.CurrentProtocol} {
		selected, err := (multiversion.Config{MinimumProtocol: floor}).ProtocolsWithRegistries(registry, items)
		if err != nil {
			t.Fatalf("floor %d: %v", floor, err)
		}
		snapshot.Config[fmt.Sprint(floor)] = protocolIDs(selected)
	}
	return snapshot
}

// TestCatalogueGolden compares the served catalogue against the recorded
// golden file. Run with GOMV_RECORD_CATALOGUE=1 to re-record it intentionally.
func TestCatalogueGolden(t *testing.T) {
	got := catalogueSnapshotOf(t)
	encoded, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded, '\n')
	path := filepath.Join("testdata", catalogueGoldenFile)
	if os.Getenv("GOMV_RECORD_CATALOGUE") != "" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, encoded, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("recorded %s", path)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden catalogue: %v", err)
	}
	if !bytes.Equal(want, encoded) {
		t.Fatalf("catalogue changed for %s:\n got: %s\nwant: %s", catalogueGoldenFile, encoded, want)
	}
	for name, ids := range map[string][]int32{
		"ProtocolsWithRegistries":    got.ProtocolsWithRegistries,
		"ProtocolsWithBlockRegistry": got.ProtocolsWithBlockRegistry,
	} {
		seen := make(map[int32]bool, len(ids))
		for _, id := range ids {
			if seen[id] {
				t.Fatalf("%s advertises duplicate protocol %d", name, id)
			}
			seen[id] = true
		}
	}
}
