# Overnight bug report

Completed: 2026-11-22 12:45   Last updated: 2026-11-22 12:45
Status: **INCOMPLETE** - one completion-gate item is unmet: no independent reviewer
could be run in this runtime, so `docs/completion-review.md` is an adversarial
self-review. Everything else in the gate passes.
Found: 6   Fixed: 6   Could not reproduce: 0   Not fixed: 0   Blocked: 2
(real Mojang clients, third-party client-library replay)

## Completion run summary

Six bugs were found and six fixed, each with its own failing test and its own
commit on `origin/overnight-fixes`. Three of them came from the new deep-fuzz
pass rather than from the hand-written samples.

In plain language, the changes are:

1. A client on 2168/2169 that clicked an item in its inventory no longer gets
   disconnected; the stack request inside its auth input decodes.
2. Players on 1.16.100 now see other players' block changes without reloading
   the chunk, because the sub-chunk batch is rewritten into per-block updates.
3. A player or NPC without a skin colour no longer breaks the connection on
   419/475/486.
4. Four separate "eight bytes ask for gigabytes" decoders are gone: oversized
   wire slice lengths, texture-pack and player-list counts, and the legacy
   entity metadata map are all checked before anything is allocated.

The three that matter most:

1. **The legacy metadata map (419/475/486).** An eight-byte `SetActorData`
   payload made the decoder size a map from the wire count. The 486 adapter
   allocated 1.79 GB for that payload, and the 475 adapter grew the heap past
   20 GB until the OS killed the fuzz worker - silently, with no Go panic and no
   crasher file. `TestOversizedMetadataCountDoesNotAllocate` now proves the
   decode stays under 32 MB and fails with the 1.79 GB figure if the guard is
   removed.
2. **Texture-pack and player-list counts.** A `ResourcePacksInfo` payload with a
   texture-pack count of 0xffff exhausted 24 GB. The same pattern existed in
   player lists, skins, command tables, block actions and recipe grids.
3. **The 1.9 TB slice.** A 13-byte `SubChunk` packet asked for a 1.9 TB
   allocation and killed the process with a fatal out-of-memory error instead of
   reporting a protocol error.

Real-client work: still none. Nothing here was run against a Mojang client, and
no third-party client library replay was possible; `docs/real-client-testing.md`
carries the per-version checklist. The new loopback harness joins 13 advertised
configurations through a real RakNet listener, which is a protocol-level check,
not a client check.

What was actually run on the final tree: `go build ./...`, `go vet ./...`,
`go test -p 1 ./...`, `go test -race -short -p 1 ./protocols/... ./internal/... .`,
the five-minute four-worker `-race` soak, `MV_LOOPBACK=1 go test -run
TestLoopbackJoin`, `git diff --check`, and `actionlint` on both workflows. Seven
fuzz targets each finished three consecutive ten-minute cycles at
`-parallel=1`, the resource limit for this machine.

Gate status: (a) every item done or blocked - met; (b) build, vet, test, race,
diff and oracles - met; (c) three clean cycles per fuzz target - met; (d) an
independent reviewer - **not met**, no reviewer agent exists in this runtime;
(e) everything pushed and CI configuration validates - met.

| # | Protocols | Symptom | Cause | Commit | Status |
|---|-----------|---------|-------|--------|--------|
| 1 | 419, 475, 486 | A skin with no colour could not be decoded, so the adapters failed to read their own PlayerSkin/PlayerList output | The reader rejected an empty colour string while the writer emits exactly that for an empty skin | `fix: accept an empty legacy skin colour` | fixed |
| 2 | 419 | Block changes sent as UpdateSubChunkBlocks never reached a 1.16.100 client | Protocol 419 has no sub-chunk batch packet; the batch was silently dropped | `fix: expand sub-chunk block batches for protocol 419` | fixed |
| 3 | 2168, 2169 | A PlayerAuthInput containing a stack request (any action or filter string) crashed the decoder | The nested native Marshal allocates slices through an interface only the directional wire readers expose | `fix: decode nested stack requests in PlayerAuthInput` | fixed |
| 4 | every 1.18+ adapter | A 9-byte SubChunk packet asked for a 1.9 TB slice and killed the process with a fatal out-of-memory error instead of a protocol error | The adapter slice helpers allocated the raw wire count | `fix: reject oversized wire slice lengths before allocating` | fixed |
| 5 | every adapter | A texture-pack count of 0xffff exhausted 24 GB; player list, skins, command tables, block actions and recipe grids had the same pattern | Raw `make([]T, wireCount)` outside the generic helpers | `fix: guard the remaining wire-supplied slice lengths` | fixed |
| 6 | 419, 475, 486 | An 8-byte `SetActorData` payload allocated 1.79 GB on 486 and grew the 475 worker past 20 GB until the OS killed it, taking the fuzz run down with no crasher file | The legacy adapters sized the metadata map with the wire count before reading any entry | `fix: bound the legacy entity metadata map size` | fixed |

## Length guard: can it reject a legitimate payload?

`packetio.CheckSliceLength` caps a wire count at 65,536 elements and also
rejects a count larger than the remaining payload. The second test makes a
false rejection impossible for any real list (an element is never smaller than
one byte, so a legitimate count always fits), and the ceiling sits well above
every field it guards:

| Guarded field | Where | Largest a real server sends | Margin to 65,536 |
|---|---|---|---|
| entity metadata entries | 419/475/486 `entities.go` | under 200 (vanilla uses fewer than 100 keys) | 300x |
| texture packs | 419/475/486 `resource_packs.go` | 64; the wire count is `uint16`, so 65,535 is the type maximum | 1,000x |
| shaped recipe grid | 419/475/486 `recipes_v486.go` | 9 cells (3x3) | 7,000x |
| player list entries | 419/475/486 `skins.go` | 100 on a large server | 650x |
| ability layers | every modern `shared_types.go` | 3; the wire count is `uint8` | 21,000x |
| skin animations | every modern `shared_types.go` | about 32 per skin | 2,000x |
| persona pieces | every modern `shared_types.go` | 256 | 256x |
| persona tints | every modern `shared_types.go` | about 32 | 2,000x |
| command enum values | 827/844 families `commands_v*.go` | about 2,000 (the block value list) | 32x |
| chained subcommand offsets | 827/844 families `commands_v*.go` | under 32 | 2,000x |

No site needed a raised limit. `TestLargeLegalPayloadsDecode` (419),
`TestLargeShapedRecipeGridDecodes` (486), `TestLargeCommandEnumDecodes` and
`TestLargePersonaPieceListDecodes` (748) decode those maximum sizes through the
guarded path and assert success. `TestCheckSliceLengthCallsAreReadGated` walks
the tree with `go/ast` and fails if any of the 64 call sites is outside an
`if io.reading` block; `CheckSliceLength` also returns immediately for
`*protocol.Writer` now, which is a behaviour change only for the encode path,
where the guard was never meant to apply. `TestCheckSliceLengthIgnoresWrites`
and `TestCheckSliceLengthRejectsTruncatedRead` pin both directions.

## Missing-mapping fallbacks (audit)

Every adapter funnels native identifiers through the two mappers in
`mapping/`, so the audit is much smaller than the roughly 400 adapter call
sites suggest: the report belongs in the mappers and every call site inherits
it. `mapping.ReportFallback` writes one record per (protocol, kind, name) per
process, at `slog` info level by default; `mapping.SetFallbackLogger` replaces
the sink and a nil sink restores the default, so no adapter requires a logger.

| Site | Fallback | Logged before this change? | Now |
|------|----------|----------------------------|-----|
| `BlockMapper.NativeToTarget` (82 adapter call sites) | historical air, or the opt-in substitution target | no | one record naming the protocol, the native block and the state used |
| `BlockMapper.MapNative` (44 call sites) | the same, with `exact=false` | no | the same record; the rate limit is shared with the row above |
| nil block mapper (an adapter with no registry configured) | runtime ID 0 | no | still unlogged: with no mapper there is no name, protocol or target to report |
| `ItemMapper.NativeToTarget` (137 call sites) | none: the caller rejects the packet or item, no placeholder is substituted | no | one record per (protocol, item name) with `used=dropped` |
| `ItemMapper.TargetToNative` | the caller rejects a target-only item serverbound | no | unchanged: a target-only item is not a native fallback |
| `mapping.Biome419` | a stand-in from the same family, or plains for unknown IDs | yes, one `slog.Info` per distinct source ID | unchanged |
| `mapping.Pre12650Biome` | total mapping (195 to 4, everything else unchanged) | n/a | n/a |
| entity identifiers | no mapping table exists; entity type names pass through unchanged | n/a | n/a |

Tests: `TestBlockMapperReportsFallbackOnce` (five hits, one record),
`TestReportFallbackOncePerKey` (the rate-limit key), and
`TestUnmappedSubChunkBlockStillSendsAndLogsOnce`, which sends three entries of a
block the 419 palette cannot represent, asserts three `UpdateBlock` packets
still reach the client with the air runtime ID, and asserts exactly one report
for protocol 419.

## Notes

1. **Empty legacy skin colour (419 / 475 / 486).** A player or NPC whose skin has no colour (NameTagOnly entries, servers that never set a skin) is written with an empty colour string. Reading that same value back raised "invalid value for skin colour" and the connection was dropped, so the adapter could not parse its own output. The reader now treats an empty string as the zero colour, exactly as the 1.21+ adapters already did for `""` and `"0"`.

2. **Sub-chunk block batches for 1.16.100 (419).** Modern servers (Dragonfly among them) send block changes in `UpdateSubChunkBlocks`. That packet belongs to the 1.18.00 sub-chunk request system, so the protocol-419 packet table has no entry for it and every batch was dropped: a 1.16.100 player saw other players' block changes only after reloading the chunk. The adapter now expands each batch into the per-block `UpdateBlock` packets 1.16 understands, keeping the second layer in `UpdateBlock`'s layer field and synced transitions in `UpdateBlockSynced`.

3. **Nested stack request inside PlayerAuthInput (2168 / 2169).** When a modern client bundles an item stack request in its auth input (every action it performs, filters included), the target decoder panicked with "index out of range" because the nested native encoder was given the bare wire object, which hides the slice-allocation interface. The connection was dropped, so the click never applied. The nested encoder now receives the directional reader/writer, the same way the neighbouring block-action list already did.

Found by the new conversion fuzzer, not by the sample harness, so it is a good example of why the fuzz targets exist.

4. **Oversized wire slice lengths (every 1.18+ adapter).** A 13-byte protocol-1001 `SubChunk` payload declared 0xffffffff entries. The decoder passed that straight to `make([]SubChunkEntry, count)`, so the runtime tried to reserve 1.9 TB and terminated the process with `fatal error: out of memory`; a connection can recover a panicked protocol error, but it cannot recover a fatal allocation. Every duplicated slice helper now calls gophertunnel's `FuncSliceOfLen`, which checks the declared length against the remaining payload before allocating. Found by `FuzzAdapterDecode`.

5. **The same pattern outside the generic helpers (every adapter).** After the first fix the fuzzer found a second path: a `ResourcePacksInfo` texture-pack count of 0xffff grew the worker to 24 GB. Player lists, ability layers, skin animations and persona pieces, command enum indices and shaped-recipe grids all allocated a wire count directly. `packetio.CheckSliceLength` now applies gophertunnel's payload check and a 65536 element ceiling before each of those allocations. Found by the same fuzz target on its next run.

6. **Entity metadata map sized from the wire count (419 / 475 / 486).** The legacy decoder read a metadata entry count and passed it straight to `make(protocol.EntityMetadata, count)`. An eight-byte `SetActorData` payload decoded to a count of about 800 million, so the runtime started building a map that large; the heap reached 21 GB and the OS killed the worker. That kind of death is silent: no Go panic, no crasher file, just the fuzzer's worker disappearing. A watchdog around the fuzz target captured the input, the count now goes through `packetio.CheckSliceLength` like the other wire lengths, and the input is both a regression case and the saved crasher `internal/compat/testdata/fuzz/FuzzNestedFields/5899d230f1353c18`. The 486 adapter had the same decoder; it allocated 1.79 GB from the same eight bytes, which `TestOversizedMetadataCountDoesNotAllocate` now pins to a 32 MB budget (commit `5858951`, crasher `set-actor-data-metadata-count-486`). Found by `FuzzNestedFields`.

## Baseline (Phase 1)

| Check | Result |
|-------|--------|
| `go build ./...` | pass |
| `go vet ./...` | pass |
| `go test ./...` | pass |
| `go test -race ./...` | pass (all packages) |
| `gofmt -l` | flags ~479 untouched files: the checkout uses CRLF, not a code problem |
| Wire oracles (`tools/oracle_v*`, adapter `testdata`) | pass |

Environment note: this machine sets `GOFLAGS=-tags=legacytransport` globally, which makes the default build fail because `protocols/v1_16_100/encryption_legacy.go` needs the transport fork. Every command in this session ran with `GOFLAGS="-tags="`; the repository itself is unaffected.

## Test matrix (Phase 2)

`internal/compat` is a test-only package that drives every catalogue protocol (2225, 2193, 2169, 2168, 1001, 975, 944, 924, 898, 844, 827, 766, 748, 486, 475, 419) through:

- `TestZeroValuePacketSweep`: every pool entry is written, read back and re-written. Only runtime panics (index out of range, nil dereference) fail; protocol errors on a zero value are logged, because production never writes a packet without converting it first.
- `TestConversionNoPanicSweep`: every native packet is converted in both directions, and every clientbound result must be writable by the target pool. Packets whose zero value the native pool cannot write either are skipped.
- `TestSampleRoundTrip`: 40 realistic packets (StartGame, item registry, creative content, recipes, chunks, player list/skin, actors, metadata, sounds, text, book edit, camera, scoreboard, transactions) go through conversion, target encoding, target decoding and back. The native packet must not change, the caller's input must not be mutated, and a second cycle must produce the same native packet.
- `TestDecoderTruncationSweep`: every pool entry is decoded from bounded truncated prefixes; truncation must never crash.

Two fuzz targets cover the same surface: `FuzzAdapterDecode` feeds arbitrary bytes to every adapter decoder and `FuzzConversionRoundTrip` converts whatever decodes. Run them with a fixed budget, for example `go test ./internal/compat -run XXX -fuzz=FuzzConversionRoundTrip -fuzztime=10m -parallel=2`; the reduced parallelism keeps 20 fuzz workers from exhausting memory on this machine.

Two documented exceptions live in the harness: `ItemRegistry` is clientbound-only, so adapters do not convert it back, and 419/486 deliberately drop `CraftingData` (their retail clients keep built-in recipes and crash on the current catalogue).

## Missing-block fallback (Phase 4)

Block mapping already resolves every native state in this order: exact name and states, same name with the closest states, curated substitutions (protocol 419 `MappingOptions.BlockSubstitutions`, empty by default), then a visible air-like placeholder. `TestEveryNativeBlockResolves` walks every native runtime ID for every protocol and fails if a block produces no packet, if two conversions of the same block differ, or if the target form does not come back to the same target bytes. Current fallback counts (native blocks without an exact target state): 419 13554, 486 11801, 748 5188, 1001 2080.

An old client cannot draw a block its data does not contain without a resource pack; the fallback keeps the world solid and visible instead of sending an invalid runtime ID. Only the substitution table is experimental and it stays off unless a caller passes entries.

## Decisions I made without asking

- Ran every Go command with `GOFLAGS="-tags="` because the machine's global tag breaks the default build; logged above instead of changing repository files.
- Did not treat `gofmt -l` output as a finding: the CRLF checkout makes it flag hundreds of untouched files.
- Zero-value decoding asymmetry was not counted as a bug when realistic packets round-trip byte-identically; the harness asserts stability on realistic samples and only panics on zero values.
- Kept the empty-colour fix minimal (accept `""` as zero) instead of relaxing the strict error for genuinely malformed colours, matching the newer adapters.
- Expanded 419 sub-chunk batches locally in the adapter instead of adding a packet ID to a version that never had one.
- Kept gophertunnel's compression defaults: measured `BestSpeed` saves 20% CPU for three times the bytes, and level 9 costs 28% more CPU for no gain.
- Added no chunk pacing to the library: it has no send loop, so pacing belongs to the server, and the default stays unchanged.
- Did not share a static one-element result slice between conversions: a caller that appends or reorders the result would corrupt another connection's packet.

## Blocked or skipped

- **Protocol 2168 `SetScore` GameVersion-dependent layout**: cannot be exercised without a real `*minecraft.Conn` (its client data is not constructible from tests). Marked unverified.
- **PrismarineJS bedrock-protocol replay**: not available in this environment, so no independent third-party client library check was performed.
- **Real Mojang clients**: cannot be run here; `docs/real-client-testing.md` lists the per-version checklist, platforms and bug template.
- **Fuzzing**: `FuzzAdapterDecode` and `FuzzConversionRoundTrip` ran 45 s and 90 s cycles with `-parallel=2` and no crashes after the fix. With default parallelism (20 workers) this machine runs out of memory while fuzzing, which shows up as a hung worker rather than a library bug.

## Still needs a real client

- 419/475/486 skin rendering for empty-skin players (the fix keeps the connection alive; the client still draws its default skin).
- 419 block changes from sub-chunk batches (block appearance, second layer such as waterlogged blocks, falling-block transitions).
- Everything in `docs/real-client-testing.md`.

## Smoothness (HUNT 6)

Client frame rate cannot be measured in this environment, so nothing below reports FPS. The
measurements cover what the library controls: conversion latency and allocations, bytes put on
the wire, chunk work per joining player, and what a mixed-version soak does to the heap, the GC
and the locks.

Method: `smoothness_test.go` in `package multiversion_test` builds the full default catalogue and
one full overworld chunk (24 sections, real native runtime IDs, 24 biome sections) per protocol.
`gophertunnel` appends the native 2225 model per connection, so the catalogue has 15 adapters and
there is no 2225 adapter row. Reproduce with:

```
go test -run 'TestSmoothnessLatency|TestSmoothnessJoinCost|TestSmoothnessScenes' -v .
go test -run XXX -bench 'BenchmarkSmoothnessConversion|BenchmarkSmoothnessChunk' -benchmem .
MV_SMOOTHNESS_SOAK=10 MV_SMOOTHNESS_WORKERS=24 GODEBUG=gctrace=1 go test -run TestSmoothnessSoak -v .
```

The per-packet timer on this Windows machine ticks in ~250 ns steps, so a p50 of `<0.25 µs` means
"below one clock tick" and 250 ns is the smallest reportable value. Batches of 2048 conversions
keep timer overhead out of the per-op figure; the `ns/op` column comes from `-benchmem`.

### Per-packet conversion (after the fixes below)

| Protocol | `text` p50 / p99 | `inventory_content` p50 / p99 | `text` ns/op | B/op | allocs/op |
|---|---|---|---|---|---|
| 2168 | <0.25 µs / 0.26 µs | 3.01 µs / 4.08 µs | 103.3 | 64 | 4 |
| 1001 | <0.25 µs / 0.26 µs | 2.87 µs / 20.32 µs | 118.9 | 48 | 3 |
| 975 | 0.25 µs / 0.51 µs | 5.85 µs / 15.46 µs | 106.2 | 48 | 3 |
| 944 | <0.25 µs / 0.27 µs | 3.37 µs / 7.41 µs | 128.1 | 48 | 3 |
| 924 | <0.25 µs / 0.26 µs | 3.06 µs / 5.63 µs | 106.2 | 48 | 3 |
| 898 | <0.25 µs / 0.28 µs | 3.27 µs / 4.91 µs | 121.1 | 72 | 4 |
| 844 | 0.25 µs / 0.26 µs | 3.29 µs / 4.58 µs | 133.1 | 72 | 4 |
| 827 | 0.25 µs / 0.28 µs | 3.35 µs / 13.03 µs | 143.9 | 72 | 4 |
| 766 | 0.25 µs / 0.29 µs | 3.32 µs / 5.90 µs | 148.4 | 72 | 4 |
| 748 | 0.25 µs / 0.27 µs | 3.31 µs / 5.08 µs | 128.3 | 72 | 4 |
| 486 | 0.25 µs / 0.28 µs | 3.85 µs / 5.11 µs | 105.2 | 64 | 3 |
| 475 | 0.25 µs / 0.26 µs | 3.79 µs / 6.63 µs | 134.6 | 80 | 4 |
| 419 | 0.25 µs / 0.28 µs | 3.55 µs / 5.08 µs | 114.7 | 80 | 4 |
| 2169 | 0.25 µs / 0.28 µs | 3.25 µs / 4.81 µs | 85.9 | 48 | 3 |
| 2193 | <0.25 µs / 0.25 µs | 3.05 µs / 5.11 µs | 36.5 | 16 | 1 |

`inventory_content` carries 36 item stacks and dominates the per-packet cost; a full 36-slot
window at ~3 µs is 12,000 window updates per core-second, far above what one player triggers.
The one-digit `ns/op` entries are pass-through paths that allocate only the result slice.

### Chunk conversion (before → after)

| Protocol | Before | After | Change | allocs before → after |
|---|---|---|---|---|
| 2168 | 234,354 ns | 27,958 ns | −88.1% | 101 → 53 |
| 1001 | 230,099 ns | 25,624 ns | −88.9% | 101 → 53 |
| 975 | 231,076 ns | 31,241 ns | −86.5% | 101 → 53 |
| 944 | 230,024 ns | 52,911 ns | −77.0% | 101 → 53 |
| 924 | 230,336 ns | 40,786 ns | −82.3% | 101 → 53 |
| 898 | 275,698 ns | 32,746 ns | −88.1% | 101 → 53 |
| 844 | 449,321 ns | 37,124 ns | −91.7% | 101 → 53 |
| 827 | 458,885 ns | 38,864 ns | −91.5% | 101 → 53 |
| 766 | 442,259 ns | 34,392 ns | −92.2% | 101 → 53 |
| 748 | 451,947 ns | 36,890 ns | −91.8% | 101 → 53 |
| 486 | 413,554 ns | 17,832 ns | −95.7% | 100 → 52 |
| 475 | 445,039 ns | 23,674 ns | −94.7% | 100 → 52 |
| 419 | 446,888 ns | 18,612 ns | −95.8% | 100 → 52 |
| 2169 | 493,422 ns | 43,409 ns | −91.2% | 101 → 53 |
| 2193 | 478,311 ns | 38,911 ns | −91.9% | 101 → 53 |

Bytes allocated per chunk fell from 158,604 to 109,452 (101,260 → 52,108 for the older
adapters). A whole 441-chunk view distance dropped from 189–207 ms of conversion per joining
player to 8.3–25.8 ms; per protocol: 2168 15.5, 1001 25.8, 975 12.4, 944 21.9, 924 14.1,
898 18.6, 844 12.5, 827 19.8, 766 13.0, 748 16.2, 486 8.3, 475 9.4, 419 12.4, 2169 14.6,
2193 13.5 ms.

### Bytes and time per joining player (baseline)

| Protocol | login bytes | 441 chunks (bytes) | 441 chunks (time) |
|---|---|---|---|
| 2168 | 108,436 | 11,020,149 | 202.1 ms |
| 1001 | 107,379 | 11,020,149 | 201.7 ms |
| 975 | 107,493 | 11,020,149 | 203.9 ms |
| 944 | 106,278 | 11,020,149 | 200.7 ms |
| 924 | 106,206 | 11,020,149 | 192.4 ms |
| 898 | 106,073 | 11,020,149 | 202.7 ms |
| 844 | 92,033 | 11,020,149 | 202.8 ms |
| 827 | 89,534 | 11,020,149 | 194.1 ms |
| 766 | 52,403 | 11,020,149 | 207.4 ms |
| 748 | 51,208 | 11,020,149 | 201.8 ms |
| 486 | 30,292 | 10,988,397 | 190.1 ms |
| 475 | 29,992 | 10,988,397 | 189.9 ms |
| 419 | 820,844 | 10,988,397 | 201.6 ms |
| 2169 | 108,436 | 11,020,149 | 201.1 ms |
| 2193 | 186,491 | 11,020,149 | 194.1 ms |

Protocol 419 needs a 820 KB login because 1.16.100's `StartGame` embeds its complete block palette;
the client needs every block state it may draw, so that is required data, not waste. The 11 MB
chunk area is the fixture's raw size before compression; a uniform test chunk is a worst case for
the “bytes” column, not a typical world.

### Sustained packet rate by scene (conversions per second, one core)

| Protocol | movement | building | entity-heavy |
|---|---|---|---|
| 2168 | 2,620,424 | 1,417,136 | 1,163,799 |
| 1001 | 3,502,694 | 1,267,903 | 1,310,298 |
| 975 | 3,217,305 | 1,302,186 | 1,137,978 |
| 944 | 2,318,219 | 1,270,970 | 1,182,975 |
| 924 | 3,646,126 | 1,181,074 | 1,082,194 |
| 898 | 2,830,071 | 1,271,163 | 1,107,209 |
| 844 | 2,101,333 | 892,702 | 1,066,392 |
| 827 | 2,965,903 | 1,155,168 | 942,334 |
| 766 | 1,253,313 | 688,240 | 809,911 |
| 748 | 2,588,042 | 942,518 | 784,709 |
| 486 | 1,973,567 | 1,152,667 | 667,159 |
| 475 | 2,001,828 | 1,072,963 | 807,563 |
| 419 | 2,092,167 | 621,302 | 653,536 |
| 2169 | 3,171,512 | 1,365,545 | 1,205,077 |
| 2193 | 3,967,574 | 1,424,184 | 3,647,978 |

A movement packet is one per client tick (20/s), so even the slowest adapter has three orders of
magnitude of headroom on one core. Building and entity scenes cost more because every update
carries a runtime ID and, for inventory actions, an item stack.

### Payload inflation

Conversion can add fixed fields (a layer byte, a filter string) but must not duplicate data. Worst
case per protocol over the measured packets:

| Protocol | Packet | Native bytes | Target bytes | Growth |
|---|---|---|---|---|
| 2168 | update_block | 7 | 9 | +28.6% |
| 1001 | update_block | 7 | 9 | +28.6% |
| 975 | update_block | 7 | 9 | +28.6% |
| 944 | update_block | 7 | 8 | +14.3% |
| 898 | text | 22 | 48 | +118.2% |
| 2169 | update_block | 7 | 9 | +28.6% |
| 2193 | update_block | 7 | 9 | +28.6% |

Every other protocol and fixture packet was equal or smaller after conversion. The eight-byte
`update_block` growth is the second-layer byte older clients expect. The `text` growth on 898 is
the legacy filter fields the historical wire carries; no payload was found with duplicated
metadata or oversized block-entity NBT.

### Compression tradeoff (one varied 110 KB chunk)

| flate level | Compressed | Ratio | Time |
|---|---|---|---|
| 1 (`BestSpeed`) | 16,378 B | 14.8% | 206 µs |
| 6 (`Default`) | 5,525 B | 5.0% | 256 µs |
| 9 (`BestCompression`) | 5,525 B | 5.0% | 327 µs |

`BestSpeed` saves about 20% CPU but sends three times the bytes; level 9 costs 28% more CPU for
no size gain on this data. No default was changed: the adapters select the historical algorithm
(`LegacyNetworkSettings` returns flate for 419/475/486) and gophertunnel owns the level. A server
owner who wants the bandwidth/CPU tradeoff differently can set it there.

Threshold: gophertunnel defaults to 256 bytes (`minecraft.ListenConfig.CompressionThreshold`). The
movement, block and chat packets measured here are 7–48 bytes, so they skip flate entirely, while
the 25 KB chunk payload always compresses. Raising the threshold would only delay the point where
a ~3 KB inventory window starts saving bytes; lowering it would spend CPU on 9-byte updates. The
default is left unchanged.

### Soak: 24 mixed-version workers, 10 seconds

| Metric | Value |
|---|---|
| Rounds (8 conversions + 1 chunk each) | 729,290 (72,929/s) |
| Live heap | 129.4 MB → 129.5 MB (stable) |
| Total allocated | 72.5 GB (7.2 GB/s, deliberate stress) |
| GC cycles | 21 → 417 |
| GC pause total / longest | 174 ms (1.7% of wall) / 1.57 ms |
| Goroutines | 2 → 2 (no leak) |
| Mutex profile | 100% runtime locks (`_LostContendedRuntimeLock` 72.5%, `runtime.unlock` 27.5%) |
| Block profile | only `sync.WaitGroup.Wait` at shutdown |

No library mutex appears in the contention profile; the delay is the Go runtime allocator under an
intentionally extreme allocation rate (one whole chunk per round, which no relay does). Heap in
use returns to its starting value after a forced GC, and no goroutines are left behind.

### Performance kept

| What | Before | After | Change |
|---|---|---|---|
| Clientbound conversion on 748/766/827/844 (`text`) | 30.0–33.6 µs | 0.25 µs measured / 128–148 ns/op bench | ≈99% lower |
| Chunk conversion, one overworld chunk | 230–493 µs, 101 allocs, 158 KB/op | 17.8–52.9 µs, 52–53 allocs, 109 KB/op | 6–13× faster (−77% to −96%) |
| 441-chunk view-distance area | 189–207 ms | 8.3–25.8 ms | 8–15× faster (−88% to −96%) |

The conversion fix caches the target packet-ID set instead of rebuilding the whole packet pool to
answer “does this version have this packet?” once per relayed packet. The pool build allocated one
constructor closure per entry, which is why every clientbound packet on those four versions cost
more than 30 µs while the same packet cost 0.25 µs on 2168/2193.

The chunk fix notices when a palette's entries keep their positions (the normal case: distinct
blocks each map to a distinct target state) and then only rewrites the palette values instead of
walking and repacking all 4096 cell indices. Width changes, duplicate mapped values and padded
palettes still take the repacking path, and out-of-range indices are still rejected. The golden
file and every existing chunk test produce the same bytes before and after.

### Chunk send pacing

A join converts 441 chunks. Before the fast path that was 189–207 ms of CPU; after it, 8.3–25.8 ms.
The remaining cost is a 25 KB output copy and palette parsing, not the per-cell repack.
The library itself has no send loop, so chunk order and rate are chosen by the server (Dragonfly
or the caller's proxy). No pacing code was added and default behaviour is unchanged. If a server
writes all 441 payloads in one tick it still puts ~11 MB before compression on the wire, so the
mitigation belongs in its scheduler: nearest chunks first and a bounded number per tick. The
`TestSmoothnessJoinCost` harness can be reused to verify such a scheduler.

### Needs a real device

FPS and hitching are client-side and were not measured here. To compare a build before and after
this work, use the same world, the same spot, the same render distance and the same resource packs,
and enable the platform's on-screen FPS overlay:

- **Windows (Bedrock, F1 overlay)**: enable *Settings → Creator → Show debug info* or the F1
  overlay; stand still for 60 s, walk through a village, then place and break 100 blocks; note FPS
  low, average and 1% low, and any freeze longer than 100 ms.
- **Android**: enable *Settings → Creator → Show debug info*; repeat the same three minutes on
  the same device, ideally with the charger connected to keep clocks stable.
- **iOS/iPadOS**: the same creator overlay; record device model and whether Low Power Mode is on.
- **Xbox / PlayStation / Switch**: no overlay without developer mode; use a capture card or the
  console's built-in performance overlay if available and compare recorded frame times.

Results vary by device, GPU, thermal state and world content; a desktop result says nothing about a
phone. Compare builds on the same device, alternating them twice, and treat a difference under 5%
as noise.
