# Completion ledger

Started: 2026-11-22 (completion run)   Last updated: 2026-11-22

Status: INCOMPLETE - gate (d) is unmet: no independent reviewer can be spawned in this runtime, so `docs/completion-review.md` is an adversarial self-review. Every other gate item is met.

This ledger is the single source of truth for the completion run on branch
`overnight-fixes`. An item is `done` only when the evidence column names a
commit, a test that was actually run, a benchmark number, or a log line. Claims
without evidence stay `todo`. Items that cannot be executed in this environment
are `blocked` after four documented attempts with different approaches.

Legacy protocols 419/475/486 are opt-in through the `legacytransport` build tag
and still require the transport fork; that restriction is a repository decision,
not a defect, and is recorded in `AGENTS.md`.

## Gate

| Gate | Requirement | Status |
|---|---|---|
| a | Every item done with evidence or blocked after 4 attempts; no todo/in-progress | MET |
| b | build, vet, test, race, diff check and oracles pass on the final commit | MET |
| c | Every fuzz target 3 consecutive clean 10-minute cycles at `-parallel=1` (resource limits for this run) | MET |
| d | Independent reviewer writes `docs/completion-review.md` and finds nothing new | NOT MET |
| e | Everything pushed to `origin/overnight-fixes`; CI configuration validates | MET |

## Items

| ID | Item | Status | Evidence |
|---|---|---|---|
| L01 | Branch is `overnight-fixes`, tree clean, no unpushed commits at start | done | `git status --short` empty, `git log origin/overnight-fixes..HEAD` empty, HEAD `f0b6340` |
| L02 | Bug 1: legacy empty skin colour accepted (419/475/486) | done | commit `c576e1d`; adapter skin tests |
| L03 | Bug 2: sub-chunk block batches expanded for 419 | done | commit `89bd30d`; protocol-419 expansion test |
| L04 | Bug 3: nested stack requests decoded in PlayerAuthInput (2168/2169) | done | commit `f0a02d3`; `TestPlayerAuthInputUsesProtocol2169BaseLayout`, compat sweep |
| L05 | Compat sweeps cover every advertised protocol | done | `internal/compat` runs the 15 adapter rows; protocol 2225 is gophertunnel's own native model, covered by its own pool and by `nativeUnwritable` in `sweep_test.go`. `go test ./internal/compat` passes (18.8 s, log `.fuzzlogs/fix2-test.log`) |
| L06 | Both conversion directions tested for every packet that differs from native | done | `TestConversionNoPanicSweep` converts every native packet to every adapter and back, and `TestSampleRoundTrip` asserts byte-identical stability in both directions for the realistic packets; the sweep only asserts no panic on zero values, which the report says explicitly. Recorded this way rather than claiming byte equality for every packet |
| L07 | Every nested Marshal/Unmarshal uses the directional reader/writer | done | audited every `protocol.NewReader/NewWriter` and nested `Marshal` call site: sub-buffers in `items.go` build their own reader from a length-prefixed byte slice (correct), and the remaining bare `Marshal(io)` calls (`placeInContainerAction`) contain primitives only. All nested types with slices go through `io.directional()` (`stack_requests.go`, `shared_types.go`, `packets_basic.go`, `packets_v827.go`) |
| L08 | Every dropped or zeroed packet is justified from Mojang docs or implemented | done | drops are limited to: packet absent from the target pool (`outgoingPacketIDs` check in every 1.21+/1.26 adapter), `packetconv.UnsupportedNativePacket` (native-only IDs above the target era, plus boss events the target lacks), `legacyOnlyPacket766` in 748/766 (a 766-only packet), 419/486 `CraftingData` (built-in recipe clients), and the 944 target-only spawn egg; all are documented in the report's "Test matrix" and AGENTS.md |
| L09 | Legacy 419/475/486 parsers tolerate legal empty/optional values | done | audited every `InvalidValue` in the three legacy adapters: empty skin colour, empty store-offer UUID and empty descriptor name are accepted; the remaining rejections are malformed UUIDs, unknown items/blocks, negative counts and a non-zero behaviour-pack count the server cannot represent |
| L10 | Loopback harness runs at every protocol, including GameVersion variants in shared families (2168) | done | `MV_LOOPBACK=1 go test -run TestLoopbackJoin -v .` passes 13 joins in 13.4 s: 2168 as 1.26.44 and 1.26.40, 1001, 975, 944, 924, 898, 844, 827, 766, 748, 2169 and 2193. 419/475/486 need the legacy transport fork (RakNet v10 / Login-first flate), so the harness skips them unless the `legacytransport` tag is built (`loopback_legacy_on_test.go` / `loopback_legacy_off_test.go`) |
| L11 | Fuzz target for chunk/palette decoding | done | commit `06d1292`: `FuzzChunkPaletteDecode`; 3/3 clean cycles in `.fuzzlogs/deep.log` |
| L12 | Fuzz target for login/negotiation | done | commit `06d1292`: `FuzzLoginNegotiation`; 3/3 clean cycles in `.fuzzlogs/deep.log` |
| L13 | Fuzz target for batch/compression | done | commit `06d1292`: `FuzzBatchFraming`; 3/3 clean cycles in `.fuzzlogs/deep.log` |
| L14 | Fuzz target for recipes/creative | done | commit `06d1292`: `FuzzRecipeCreative`; 3/3 clean cycles in `.fuzzlogs/deep.log` |
| L15 | Fuzz target for nested-field packets | done | commit `06d1292`: `FuzzNestedFields`; the target found L33 and now holds its crasher in `testdata/fuzz/FuzzNestedFields/5899d230f1353c18` |
| L16 | Deep fuzz: 3 consecutive clean cycles per target, one target at a time | done | all seven targets finished 3/3 clean 10-minute cycles at `-parallel=1`: `FuzzAdapterDecode`, `FuzzConversionRoundTrip`, `FuzzChunkPaletteDecode`, `FuzzLoginNegotiation`, `FuzzBatchFraming` and `FuzzRecipeCreative` in `.fuzzlogs/deep.log`, `FuzzNestedFields` in `.fuzzlogs/nested_final.log` after `6125cb3` and `5858951`. The fuzz cache held roughly 5 MB of corpus per target |
| L17 | Soak with many mixed-version clients clean under `-race` | done | `MV_SMOOTHNESS_SOAK=300 MV_SMOOTHNESS_WORKERS=4 go test -race -run TestSmoothnessSoak -v .` passes: 4 workers, 15 protocols, 3,987,908 rounds (13,293/s), live heap 130.0 MB → 130.1 MB, longest GC pause 16.0 ms, no race report (`.fuzzlogs/soak.log`) |
| L18 | Benchmarks exist for every converter | done | `BenchmarkSmoothnessConversion`/`BenchmarkSmoothnessChunk` iterate the whole catalogue (15 adapters, no 2225 row because gophertunnel appends the native model) across the conversion families; `chunkconv/bench_test.go` covers sub-chunk and level chunk. Numbers are in the report's Performance table |
| L19 | 2168 `SetScore` GameVersion-dependent layout exercised (was blocked) | done | the loopback harness now logs in to protocol 2168 twice through a real listener, as 1.26.40 and as 1.26.44, and both joins pass (`TestLoopbackJoin/2168/1.26.40`, `TestLoopbackJoin/2168/1.26.44`), so the GameVersion-selected layout is reachable through a connection and not only through the unit seam |
| L20 | Independent third-party client library replay (was blocked) | blocked | four attempts: (1) PrismarineJS `bedrock-protocol` as a live client - not installed and no Node toolchain in scope; (2) `minecraft-data` JSON cross-check - the data is only available over the network, and adding it as a vendored snapshot is a data-provenance change this task must not make on its own; (3) no offline copy of the JSON exists in the checkout (`data/` holds Dragonfly and Mojang snapshots only); (4) a recorded third-party client capture is not available here. Recorded as blocked with the reason in the report |
| L21 | Real Mojang clients (was blocked) | blocked | cannot run in this environment; checklist in `docs/real-client-testing.md`; attempts logged in the report |
| L22 | Smoothness optimizations shipped with before/after numbers | done | commits `4f43945`, `00dfb9b`; report Performance table |
| L23 | Compression threshold and level tradeoff measured; defaults unchanged | done | `f0b6340`, report compression section |
| L24 | Chunk pacing analysed; no library change because the library has no send loop | done | report "Chunk send pacing" |
| L25 | CI configuration validates (actionlint-style check) | done | `go run github.com/rhysd/actionlint/cmd/actionlint@latest -no-color -shellcheck=` exits 0 on `.github/workflows/ci.yml` and `codeql.yml` (actionlint v1.7.12) |
| L26 | Independent reviewer round (`docs/completion-review.md`) | blocked | no reviewer agent is spawnable in this runtime: `opus-agent`, `gpt-5-agent`, `code-reviewer-multi-prompt` and `editor-multi-prompt` were each rejected as unavailable. `docs/completion-review.md` therefore records an adversarial self-review and says so in its first section; gate (d) stays unmet |
| L27 | Final report header: status, totals, top findings, real-client work | done | `docs/overnight-bug-report.md` now opens with the COMPLETE/INCOMPLETE status, found/fixed/blocked totals, the six plain-language changes, the top three findings and the real-client statement |
| L28 | Final checks: build, vet, test ./..., race on touched packages and root, diff check, oracles | done | on this tree: `go build ./...` 0, `go vet ./...` 0, `go test -p 1 ./...` 0 (`.fuzzlogs/full.log`), `go test -race -short -p 1 ./protocols/... ./internal/... .` 0 (`.fuzzlogs/race.log`), `git diff --check` 0. Oracles are part of `./...` (`TestSetScoreOraclePayloads`, protocol `testdata` goldens) |
| L29 | Everything pushed to `origin/overnight-fixes` | done | fixed and pushed per item: `6303a27`, `0bd2472`, `06d1292`, `6125cb3`, `5858951`, `8807cb6`, plus the documentation commit that carries this ledger; `git log origin/overnight-fixes..HEAD` is empty after it |
| L30 | Unbounded wire slice length caused a fatal 1.9 TB allocation (found by fuzzing) | done | commit `6303a27`; `TestOversizedWireLengthsRejected`; seed `internal/compat/testdata/fuzz/FuzzAdapterDecode/ec7faf06958d5620` reproduced `VirtualAlloc of 1954424520704 bytes failed` before the fix |
| L31 | Audit every raw `make([]T, wireCount)` in the adapters | done | `6303a27`: 21 slice-helper files plus the 1.18.10 `chunks.go`, 3 `skins.go`, 419 `items.go` and the 419/475/486 StartGame lists now call `protocol.FuncSliceOfLen`; `grep -rn 'make(\[\]T, count)' protocols` returns 0 |
| L33 | Third allocation class: legacy entity metadata map sized from the wire count (21 GB, found by `FuzzNestedFields`) | done | commits `6125cb3` (419, 475) and `5858951` (486): `packetio.CheckSliceLength` before `make(protocol.EntityMetadata, count)`; saved crashers `internal/compat/testdata/fuzz/FuzzNestedFields/5899d230f1353c18` and `set-actor-data-metadata-count-486`; `TestOversizedMetadataCountDoesNotAllocate` fails with "protocol 486 allocated 1789395976 bytes decoding a 8-byte payload" without the guard and passes with it |
| L32 | Second allocation class: texture pack / player list / skin / command counts (24 GB) | done | commit `0bd2472`; `packetio.CheckSliceLength` at every remaining site; `TestOversizedWireLengthsRejected` now covers SubChunk, PlayerList, ResourcePacksInfo and ResourcePackClientResponse |

## Decisions

- Fuzzing always uses `GOFLAGS="-tags="` because the machine's global tag breaks
  the default build.
- Fuzz cycles run one target at a time with `-parallel=1` and a 10 minute
  budget, and the fuzz cache is cleared between targets: this machine exhausted
  memory with concurrent workers, and these are the resource limits for this run.
  Three consecutive clean cycles per target are still required.
- Legacy protocols stay behind `legacytransport`; the loopback harness covers
  them only when that tag and the transport fork are in use, and the default
  loopback covers the advertised families.
