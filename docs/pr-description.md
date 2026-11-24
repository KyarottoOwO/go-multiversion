# Go multiversion: legacy fixes, allocation guards, fuzzing and chunk speedups

## What changed

**Three connection bugs.**

- A skin with no colour could not be read back by the 419/475/486 adapters, so a
  player or NPC without a skin broke the connection. Empty now means the zero
  colour; malformed colours still error.
- Protocol 419 has no `UpdateSubChunkBlocks` packet, so every block-change batch
  was dropped and 1.16.100 players saw other players' blocks only after a chunk
  reload. Batches are expanded into `UpdateBlock`/`UpdateBlockSynced`.
- A stack request inside `PlayerAuthInput` crashed the 2168/2169 decoder because
  the nested encoder lost the slice-allocation interface. The nested encoder now
  gets the directional reader/writer.

**Three allocation fixes, all found by fuzzing.**

- A 9- to 13-byte `SubChunk` packet asked for a 1.9 TB slice and killed the
  process with a fatal out-of-memory error instead of a protocol error. Slice
  decoding now goes through gophertunnel's guarded helper.
- Texture-pack (`0xffff` grew the heap to 24 GB), player-list, skin, command,
  block-action and recipe-grid counts had the same pattern. A shared
  `packetio.CheckSliceLength` adds a 65,536-element ceiling and a payload check
  to 64 call sites in 27 files.
- The legacy entity metadata map was sized from the wire count: eight bytes
  allocated 1.79 GB on 486 and grew the 475 fuzz worker past 20 GB until the OS
  killed it, silently. The count is guarded now and both payloads are saved
  fuzz crashers.

**Tests and fuzzing.** Seven fuzz targets (adapter decode, conversion round
trip, chunk palettes, login negotiation, batch framing, recipes/creative, nested
fields), a loopback harness that joins all 13 advertised configurations through
a real RakNet listener, allocation-budget tests, and tests for every new guard.

**Two speedups, with numbers.** Chunk conversion per overworld chunk fell from
230-493 us to 17.8-52.9 us (101 -> 52-53 allocations, 158,604 -> 109,452 bytes);
a 441-chunk view distance went from 189-207 ms to 8.3-25.8 ms. Clientbound
`text` conversion on 748/766/827/844 fell from 30.0-33.6 us to 0.25 us measured
(128-148 ns/op). The first comes from skipping the palette repack when entries
keep their positions; the second from caching the target packet-ID set.

**Also new:** unmapped blocks and items are reported once per protocol and name
through `mapping.SetFallbackLogger` (default: one `slog` record), instead of
disappearing silently. The placeholder policy itself is unchanged.

## Verified automatically

`go build ./...`, `go vet ./...`, `go test -p 1 ./...`, a `-race` run over
`protocols/`, `internal/` and the root package, a five-minute four-worker `-race`
soak (3.99M rounds, heap flat at 130 MB), the 13-configuration loopback join,
`git diff --check`, and actionlint on both workflows. Three consecutive
ten-minute fuzz cycles per target at `-parallel=1`.

## NOT verified

- No real Mojang client was run, on any version. Nothing here proves terrain,
  water, block appearance, inventory, crafting or gameplay render correctly.
- No third-party client library replay (PrismarineJS or similar) was possible.
- CI has not been observed from here; only the workflow files were linted.
- Frame rate and hitching were not measured; the performance numbers are
  conversion time, not client FPS.
- The modern shaped-grid `uint32(Width*Height)` cast can truncate; it can only
  under-allocate and fail while reading, and was left unchanged.

## Look at these first

1. `chunkconv/palette.go` - the ordered-palette fast path. It rewrites only the
   palette values when every entry keeps its position and the width is
   unchanged; a mistake here corrupts terrain rather than crashing. Compare it
   with `TestOrderedRemapMatchesRepack` and the golden chunk test.
2. `protocols/v1_16_100/gameplay_transactions.go` (`targetBlockChanges`) and
   `protocols/v1_16_100/subchunk_blocks_positions_test.go` - the 419 batch
   expansion and the absolute-coordinate proof, including negative Y.
3. `internal/packetio/slice_limit.go` and its tests - the ceiling, the payload
   check, the read-gating AST test, and the write-path short circuit.
