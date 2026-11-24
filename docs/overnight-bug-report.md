# Overnight bug report

Completed: 2026-11-22 12:45   Last updated: 2026-11-24
Status: **INCOMPLETE** - one completion-gate item is unmet: no independent reviewer
could be run in this runtime, so `docs/completion-review.md` is an adversarial
self-review. Everything else in the gate passes.
Found: 6   Fixed: 6   Could not reproduce: 0   Not fixed: 0   Blocked: 2
(real Mojang clients, third-party client-library replay)

Six bugs were found and six fixed, each with its own failing test and commit on
`origin/overnight-fixes`. Three came from the new deep-fuzz pass rather than the
hand-written samples. Real-client work: none - nothing here was run against a
Mojang client, and no third-party client library replay was possible; see
"Still needs a real client".

In plain language:

1. A client on 2168/2169 that clicked an item no longer gets disconnected; the
   stack request inside its auth input decodes.
2. Players on 1.16.100 see other players' block changes without reloading the
   chunk, because the sub-chunk batch is rewritten into per-block updates.
3. A player or NPC without a skin colour no longer breaks 419/475/486.
4. Four "eight bytes ask for gigabytes" decoders are gone: oversized wire slice
   lengths, texture-pack and player-list counts, and the legacy entity metadata
   map are checked before anything is allocated.

| # | Protocols | Symptom | Cause | Commit | Status |
|---|-----------|---------|-------|--------|--------|
| 1 | 419, 475, 486 | A skin with no colour could not be decoded, so the adapters failed to read their own PlayerSkin/PlayerList output | The reader rejected an empty colour string while the writer emits exactly that for an empty skin | `fix: accept an empty legacy skin colour` | fixed |
| 2 | 419 | Block changes sent as UpdateSubChunkBlocks never reached a 1.16.100 client | Protocol 419 has no sub-chunk batch packet; the batch was silently dropped | `fix: expand sub-chunk block batches for protocol 419` | fixed |
| 3 | 2168, 2169 | A PlayerAuthInput containing a stack request crashed the decoder | The nested native Marshal allocated slices through an interface only the directional readers expose | `fix: decode nested stack requests in PlayerAuthInput` | fixed |
| 4 | every 1.18+ adapter | A 9-byte SubChunk packet asked for a 1.9 TB slice and killed the process with a fatal out-of-memory error | The adapter slice helpers allocated the raw wire count | `fix: reject oversized wire slice lengths before allocating` | fixed |
| 5 | every adapter | A texture-pack count of 0xffff exhausted 24 GB; player lists, skins, command tables, block actions and recipe grids had the same pattern | Raw `make([]T, wireCount)` outside the generic helpers | `fix: guard the remaining wire-supplied slice lengths` | fixed |
| 6 | 419, 475, 486 | An 8-byte `SetActorData` payload allocated 1.79 GB on 486 and grew the 475 worker past 20 GB until the OS killed it, with no panic and no crasher file | The legacy adapters sized the metadata map with the wire count before reading any entry | `fix: bound the legacy entity metadata map size` | fixed |

Three of them matter most: the legacy metadata map (silent multi-gigabyte
allocation), the texture-pack and player-list counts (24 GB from one payload),
and the 1.9 TB `SubChunk` slice (fatal, not recoverable as a protocol error).

Notes, one line each:

1. **Empty skin colour**: readers now accept `""` as the zero colour, as the
   1.21+ adapters already did; a genuinely malformed colour still errors.
2. **419 sub-chunk batches**: expanded into `UpdateBlock`, second layer in
   `Layer`, synced transitions in `UpdateBlockSynced`.
3. **Nested stack requests**: the nested encoder now receives the directional
   reader/writer, like the neighbouring block-action list; found by fuzzing.
4. **Oversized slice lengths**: gophertunnel's `FuncSliceOfLen` checks the
   declared length against the remaining payload before allocating.
5. **Guarded counts**: `packetio.CheckSliceLength` adds a 65,536-element
   ceiling and the payload check to 64 call sites in 27 files.
6. **Metadata map**: the count goes through the same helper; the saved crashers
   are `internal/compat/testdata/fuzz/FuzzNestedFields/5899d230f1353c18` and
   `set-actor-data-metadata-count-486`.

## What was run

`go build ./...`, `go vet ./...`, `go test -p 1 ./...`, `go test -race -short
-p 1 ./protocols/... ./internal/... .`, a five-minute four-worker `-race` soak
(3,997,908 rounds, 13,293/s, live heap 130.0 MB -> 130.1 MB, longest GC pause
16 ms), `MV_LOOPBACK=1 go test -run TestLoopbackJoin` (13 joins), `git diff
--check`, and actionlint on both workflows. Seven fuzz targets each finished
three consecutive ten-minute cycles at `-parallel=1`.

Gate: (a) every item done or blocked - met; (b) build, vet, test, race, diff and
oracles - met; (c) three clean cycles per fuzz target - met; (d) an independent
reviewer - **not met**, no reviewer agent exists in this runtime; (e) everything
pushed and CI configuration validates - met.

## Test matrix

`internal/compat` drives all 15 catalogue protocols (2225 native plus 2193,
2169, 2168, 1001, 975, 944, 924, 898, 844, 827, 766, 748, 486, 475, 419) through
a zero-value sweep, a both-directions conversion sweep, 40 realistic sample
round trips and a truncation sweep. Two documented exceptions: `ItemRegistry`
is clientbound-only, and 419/486 deliberately drop `CraftingData`.

## Performance table

Chunk conversion per overworld chunk: 230-493 us -> 17.8-52.9 us (6-13x, -77%
to -96%), 101 -> 52-53 allocations and 158,604 -> 109,452 bytes; a 441-chunk view
distance fell from 189-207 ms to 8.3-25.8 ms. Clientbound `text` on
748/766/827/844 fell from 30.0-33.6 us to 0.25 us measured (128-148 ns/op). Byte
and time tables for all 15 adapters are reproducible with
`go test -run XXX -bench 'BenchmarkSmoothnessConversion|BenchmarkSmoothnessChunk'
-benchmem .`

The chunk gain comes from noticing when a palette's entries keep their positions
and rewriting only palette values instead of 4,096 cell indices. Width changes,
duplicate mapped values and padded palettes still repack, and out-of-range
indices are still rejected.

## Chunk send pacing

A join converts 441 chunks: 189-207 ms before the fast path, 8.3-25.8 ms after,
and about 11 MB of payload before compression. The library has no send loop, so
pacing stays with the server; no default changed.

## Compression

One 110 KB chunk: level 1 gives 16,378 B in 206 us, level 6 gives 5,525 B in
256 us, level 9 gives 5,525 B in 327 us. BestSpeed saves about 20% CPU for three
times the bytes; level 9 costs 28% more for no gain. Defaults are unchanged, and
gophertunnel still skips flate below 256 bytes.

## Missing-mapping fallbacks (audit)

Every adapter funnels native identifiers through the two mappers in `mapping/`,
so the audit is much smaller than the roughly 400 call sites suggest.
`mapping.ReportFallback` writes one record per (protocol, kind, name) per
process, at `slog` info level by default; `mapping.SetFallbackLogger` replaces
the sink and a nil sink restores the default, so no adapter requires a logger.

| Site | Fallback | Logged before this change? | Now |
|------|----------|----------------------------|-----|
| `BlockMapper.NativeToTarget` (82 call sites) | historical air, or the opt-in substitution target | no | one record naming the protocol, the native block and the state used |
| `BlockMapper.MapNative` (44 call sites) | the same, with `exact=false` | no | the same record; the rate limit is shared with the row above |
| nil block mapper | runtime ID 0 | no | still unlogged: there is no name, protocol or target to report |
| `ItemMapper.NativeToTarget` (137 call sites) | none: the caller rejects the packet or item | no | one record per (protocol, item name) with `used=dropped` |
| `ItemMapper.TargetToNative` | the caller rejects a target-only item serverbound | no | unchanged: not a native fallback |
| `mapping.Biome419` | a stand-in from the same family, or plains for unknown IDs | yes, one `slog.Info` per distinct source ID | unchanged |
| `mapping.Pre12650Biome` | total mapping (195 to 4, everything else unchanged) | n/a | n/a |
| entity identifiers | no mapping table exists; names pass through unchanged | n/a | n/a |

Tests: `TestBlockMapperReportsFallbackOnce` (five hits, one record),
`TestReportFallbackOncePerKey`, and
`TestUnmappedSubChunkBlockStillSendsAndLogsOnce`, which sends three entries of a
block the 419 palette cannot represent, asserts three `UpdateBlock` packets
still reach the client with the air runtime ID, and asserts exactly one report.

## Length guard: can it reject a legitimate payload?

The guard caps a wire count at 65,536 elements and rejects a count larger than
the remaining payload. An element is never smaller than one byte, so a
legitimate count always fits; the ceiling sits far above every guarded field:

| Guarded field | Where | Largest a real server sends | Margin to 65,536 |
|---|---|---|---|
| entity metadata entries | 419/475/486 `entities.go` | under 200 (vanilla has fewer than 100 keys) | 300x |
| texture packs | 419/475/486 `resource_packs.go` | 64; wire count is `uint16`, type maximum 65,535 | 1,000x |
| shaped recipe grid | 419/475/486 `recipes_v486.go` | 9 cells (3x3) | 7,000x |
| player list entries | 419/475/486 `skins.go` | 100 | 650x |
| ability layers | modern `shared_types.go` | 3; wire count is `uint8` | 21,000x |
| skin animations | modern `shared_types.go` | about 32 | 2,000x |
| persona pieces | modern `shared_types.go` | 256 | 256x |
| persona tints | modern `shared_types.go` | about 32 | 2,000x |
| command enum values | 827/844 `commands_v*.go` | about 2,000 (block value list) | 32x |
| chained subcommand offsets | 827/844 `commands_v*.go` | under 32 | 2,000x |

No site needed a raised limit. `TestLargeLegalPayloadsDecode` (419),
`TestLargeShapedRecipeGridDecodes` (486), `TestLargeCommandEnumDecodes` and
`TestLargePersonaPieceListDecodes` (748) decode those maximum sizes through the
guarded path. `TestCheckSliceLengthCallsAreReadGated` walks the tree with
`go/ast` and fails if any of the 64 call sites is outside `if io.reading`.
`CheckSliceLength` now also returns immediately for `*protocol.Writer`: a
behaviour change only on the encode path, where the guard was never meant to
apply. `TestCheckSliceLengthIgnoresWrites` and
`TestCheckSliceLengthRejectsTruncatedRead` pin both directions.

## Length guard: second review of the diff

Every hunk of `6303a27`, `0bd2472`, `6125cb3` and `5858951` was read again, and a
script flagged every count-sized allocation in `protocols/` without a guard; the
three hits were all `len(existing slice)` or a write-side capacity.

| Site | Check | Result |
|---|---|---|
| SubChunk entries, item strings, StartGame blocks/items, creative content (`FuncSliceOfLen`) | payload check before `make` | pass; elements are structs or strings |
| `funcSliceUint16` / `funcSliceUint32` / `funcIOSliceUint32Length` | same helper; `uint16` widened, never narrowed | pass |
| legacy metadata map | `CheckSliceLength` before `make(map, count)` | pass |
| legacy player list, texture packs | guard before `make` | pass |
| legacy shaped grid | negative and `uint64` product checked, then the cast | pass; the cast cannot wrap |
| modern shaped grid (9 adapters) | `uint32(Width*Height)` before `FuncSliceOfLen` | accepted, not changed: the same truncated value feeds the payload check and the `make`, so a wrapped product can only under-allocate (0 cells) and fail while reading; it can never over-allocate or reject a 3x3 |
| modern ability/skin/persona/command lists | guard before `make`, read-gated | pass |
| modern metadata map | no count-sized allocation; entries grow the map as bytes are read | pass by construction |

Off-by-one: both comparisons are `n > MaxSliceElements`, so exactly 65,536 is
accepted and 65,537 rejected.

## Decisions I made without asking

- Ran every Go command with `GOFLAGS="-tags="`; the machine's global
  `legacytransport` tag breaks the default build.
- Did not treat `gofmt -l` as a finding: the CRLF checkout flags untouched files.
- Kept the empty-colour fix minimal instead of relaxing the strict error.
- Expanded 419 sub-chunk batches in the adapter instead of inventing a packet ID.
- Kept gophertunnel's compression defaults after measuring the tradeoff.
- Added no chunk pacing: the library has no send loop.
- Did not share static one-element result slices between conversions.
- Reported missing mappings from the mappers, one shared helper, instead of at
  ~400 call sites, and left item misses rejected rather than substituted.
- Left the modern shaped-grid `uint32` cast alone after confirming it can only
  under-allocate.

## Blocked or skipped

- **Protocol 2168 `SetScore` GameVersion layout**: needs a real `*minecraft.Conn`.
- **PrismarineJS bedrock-protocol replay**: not available here.
- **Real Mojang clients**: cannot run here; checklist in
  `docs/real-client-testing.md`.
- **`gofmt -l`**: flags ~479 untouched files because the checkout is CRLF.

## Still needs a real client

- 419/475/486 skin rendering for empty-skin players.
- 419 block changes from sub-chunk batches, including the second layer.
- Terrain, water and special blocks on any historical version, next to a world
  built by `main`; the palette fast path is byte-identical in tests only.
- Everything in `docs/real-client-testing.md`.
