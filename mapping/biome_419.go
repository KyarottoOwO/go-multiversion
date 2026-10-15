package mapping

import (
	"log/slog"
	"sync"
)

// The 1.16.100 client resolves each per-column biome byte against its own
// hardcoded registry: the 71 classic biome IDs described by the locked
// biome definition snapshot in data/v419. A biome byte outside that set has
// no definition on the client and crashes it while it applies the chunk, so
// the connection dies as the chunk stream completes with nothing logged
// server-side. Every biome the server can emit must therefore arrive as one
// of those 71 IDs: Biome419 is total - it never reports an ID as unmappable,
// because a stand-in is always safer than a byte the client cannot resolve.

// biome419Names maps the biome names of the 1.16.100 biome definition
// snapshot to the numeric IDs the client resolves per-column biome bytes
// against. The names and IDs are the classic values, carried forward
// unchanged by the native registry, and the registry test holds this table
// against the locked blob.
var biome419Names = map[string]uint32{
	"ocean": 0, "plains": 1, "desert": 2, "extreme_hills": 3, "forest": 4,
	"taiga": 5, "swampland": 6, "river": 7, "hell": 8, "the_end": 9,
	"legacy_frozen_ocean": 10, "frozen_river": 11, "ice_plains": 12, "ice_mountains": 13,
	"mushroom_island": 14, "mushroom_island_shore": 15, "beach": 16, "desert_hills": 17,
	"forest_hills": 18, "taiga_hills": 19, "extreme_hills_edge": 20, "jungle": 21,
	"jungle_hills": 22, "jungle_edge": 23, "deep_ocean": 24, "stone_beach": 25,
	"cold_beach": 26, "birch_forest": 27, "birch_forest_hills": 28, "roofed_forest": 29,
	"cold_taiga": 30, "cold_taiga_hills": 31, "mega_taiga": 32, "mega_taiga_hills": 33,
	"extreme_hills_plus_trees": 34, "savanna": 35, "savanna_plateau": 36, "mesa": 37,
	"mesa_plateau_stone": 38, "mesa_plateau": 39, "warm_ocean": 40, "deep_warm_ocean": 41,
	"lukewarm_ocean": 42, "deep_lukewarm_ocean": 43, "cold_ocean": 44, "deep_cold_ocean": 45,
	"frozen_ocean": 46, "deep_frozen_ocean": 47, "bamboo_jungle": 48, "bamboo_jungle_hills": 49,
	"sunflower_plains": 129, "desert_mutated": 130, "extreme_hills_mutated": 131,
	"flower_forest": 132, "taiga_mutated": 133, "swampland_mutated": 134, "ice_plains_spikes": 140,
	"jungle_mutated": 149, "jungle_edge_mutated": 151, "birch_forest_mutated": 155,
	"birch_forest_hills_mutated": 156, "roofed_forest_mutated": 157, "cold_taiga_mutated": 158,
	"redwood_taiga_mutated": 160, "redwood_taiga_hills_mutated": 161,
	"extreme_hills_plus_trees_mutated": 162, "savanna_mutated": 163, "savanna_plateau_mutated": 164,
	"mesa_bryce": 165, "mesa_plateau_stone_mutated": 166, "mesa_plateau_mutated": 167,
}

// Biome419Names returns the biome names of the 1.16.100 client registry with
// the numeric IDs it resolves per-column biome bytes against. It is exported
// for tests that hold the table against the locked biome definition
// snapshot.
func Biome419Names() map[string]uint32 { return biome419Names }

// biome419Valid is the set of biome IDs the 1.16.100 client can resolve,
// built from biome419Names.
var biome419Valid = func() map[uint32]struct{} {
	valid := make(map[uint32]struct{}, len(biome419Names))
	for _, id := range biome419Names {
		valid[id] = struct{}{}
	}
	return valid
}()

// biome419Remap holds the stand-in for every biome newer than 1.16.100, by
// family: the nether biomes collapse to hell, the peaks to ice_mountains,
// the overworld newcomers to their closest classic look, and the underground
// biomes to plains, which is also the catch-all. The native runtime IDs are
// locked to the same registry as Pre12650Biome.
var biome419Remap = map[uint32]struct {
	name string
	to   uint32
}{
	178: {"soulsand_valley", 8},
	179: {"crimson_forest", 8},
	180: {"warped_forest", 8},
	181: {"basalt_deltas", 8},
	182: {"jagged_peaks", 13},
	183: {"frozen_peaks", 13},
	184: {"snowy_slopes", 12},
	185: {"grove", 30},
	186: {"meadow", 1},
	187: {"lush_caves", 1},
	188: {"dripstone_caves", 1},
	189: {"stony_peaks", 3},
	190: {"deep_dark", 1},
	191: {"mangrove_swamp", 6},
	192: {"cherry_grove", 132},
	193: {"pale_garden", 29},
	194: {"sulfur_caves", 1},
	195: {"dappled_forest", 4},
}

// biome419Logged records the source biome IDs whose remap has already been
// logged, so the substitution of each distinct biome is written down once
// per process rather than once per chunk column.
var biome419Logged sync.Map

// Biome419 maps a native biome runtime ID to one the 1.16.100 client can
// resolve: known biomes pass through unchanged, biomes newer than 1.16.100
// are substituted with a stand-in from their family, and anything the table
// does not recognise - a custom biome registered by a world, for instance -
// becomes plains. The bool returned is always true: the mapping is total by
// design, so the caller's unmapped-biome reporting never fires for 419 and
// the remap log below is the only trace of what was substituted.
func Biome419(id uint32) (uint32, bool) {
	if _, ok := biome419Valid[id]; ok {
		return id, true
	}
	if remap, ok := biome419Remap[id]; ok {
		logBiome419Remap(id, remap.to)
		return remap.to, true
	}
	logBiome419Remap(id, 1)
	return 1, true
}

// logBiome419Remap writes one line per distinct source biome ID, so a join on
// an old client shows exactly which biomes the world actually used and what
// the client saw in their place.
func logBiome419Remap(from, to uint32) {
	if _, loaded := biome419Logged.LoadOrStore(from, to); !loaded {
		slog.Info("protocol 419 biome remapped", "from", from, "to", to)
	}
}
