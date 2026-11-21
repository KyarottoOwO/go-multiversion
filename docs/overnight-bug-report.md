# Overnight bug report

Started: 2026-11-21 01:30   Last updated: 2026-11-21 05:00
Found: 3   Fixed: 3   Could not reproduce: 0   Not fixed: 0   Blocked: 1

| # | Version(s) | What went wrong (1 line) | Why (1 line) | Fix commit | Status |
|---|------------|--------------------------|--------------|------------|--------|
| 1 | 419, 475, 486 | A skin with no colour could not be decoded, so the adapters failed to read their own PlayerSkin/PlayerList output | The reader rejected an empty colour string while the writer emits exactly that for an empty skin | `fix: accept an empty legacy skin colour` | fixed |
| 2 | 419 | Block changes sent as UpdateSubChunkBlocks never reached a 1.16.100 client | Protocol 419 has no sub-chunk batch packet; the batch was silently dropped | `fix: expand sub-chunk block batches for protocol 419` | fixed |
| 3 | 2168, 2169 | A PlayerAuthInput containing a stack request (any action or filter string) crashed the decoder | The nested native Marshal allocates slices through an interface only the directional wire readers expose | `fix: decode nested stack requests in PlayerAuthInput` | fixed |

## Notes

1. **Empty legacy skin colour (419 / 475 / 486).** A player or NPC whose skin has no colour (NameTagOnly entries, servers that never set a skin) is written with an empty colour string. Reading that same value back raised "invalid value for skin colour" and the connection was dropped, so the adapter could not parse its own output. The reader now treats an empty string as the zero colour, exactly as the 1.21+ adapters already did for `""` and `"0"`.

2. **Sub-chunk block batches for 1.16.100 (419).** Modern servers (Dragonfly among them) send block changes in `UpdateSubChunkBlocks`. That packet belongs to the 1.18.00 sub-chunk request system, so the protocol-419 packet table has no entry for it and every batch was dropped: a 1.16.100 player saw other players' block changes only after reloading the chunk. The adapter now expands each batch into the per-block `UpdateBlock` packets 1.16 understands, keeping the second layer in `UpdateBlock`'s layer field and synced transitions in `UpdateBlockSynced`.

3. **Nested stack request inside PlayerAuthInput (2168 / 2169).** When a modern client bundles an item stack request in its auth input (every action it performs, filters included), the target decoder panicked with "index out of range" because the nested native encoder was given the bare wire object, which hides the slice-allocation interface. The connection was dropped, so the click never applied. The nested encoder now receives the directional reader/writer, the same way the neighbouring block-action list already did.

Found by the new conversion fuzzer, not by the sample harness, so it is a good example of why the fuzz targets exist.

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

## Decisions I made without asking

- Ran every Go command with `GOFLAGS="-tags="` because the machine's global tag breaks the default build; logged above instead of changing repository files.
- Did not treat `gofmt -l` output as a finding: the CRLF checkout makes it flag hundreds of untouched files.
- Zero-value decoding asymmetry was not counted as a bug when realistic packets round-trip byte-identically; the harness asserts stability on realistic samples and only panics on zero values.
- Kept the empty-colour fix minimal (accept `""` as zero) instead of relaxing the strict error for genuinely malformed colours, matching the newer adapters.
- Expanded 419 sub-chunk batches locally in the adapter instead of adding a packet ID to a version that never had one.

## Blocked or skipped

- **Protocol 2168 `SetScore` GameVersion-dependent layout**: cannot be exercised without a real `*minecraft.Conn` (its client data is not constructible from tests). Marked unverified.
- **PrismarineJS bedrock-protocol replay**: not available in this environment, so no independent third-party client library check was performed.
- **Real Mojang clients**: cannot be run here; see `docs/real-client-testing.md`.
- **Fuzzing**: `FuzzAdapterDecode` and `FuzzConversionRoundTrip` ran 45 s and 90 s cycles with `-parallel=2` and no crashes after the fix. With default parallelism (20 workers) this machine runs out of memory while fuzzing, which shows up as a hung worker rather than a library bug.

## Still needs a real client

- 419/475/486 skin rendering for empty-skin players (the fix keeps the connection alive; the client still draws its default skin).
- 419 block changes from sub-chunk batches (block appearance, second layer such as waterlogged blocks, falling-block transitions).
- Everything in `docs/real-client-testing.md`.
