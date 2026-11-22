# Completion ledger

Started: 2026-11-22 (completion run)   Last updated: 2026-11-22

Status: IN PROGRESS

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
| a | Every item done with evidence or blocked after 4 attempts; no todo/in-progress | IN PROGRESS |
| b | build, vet, test, race, diff check and oracles pass on the final commit | IN PROGRESS |
| c | Every fuzz target 3 consecutive clean 30-minute cycles at `-parallel=2` | IN PROGRESS |
| d | Independent reviewer writes `docs/completion-review.md` and finds nothing new | TODO |
| e | Everything pushed to `origin/overnight-fixes`; CI configuration validates | TODO |

## Items

| ID | Item | Status | Evidence |
|---|---|---|---|
| L01 | Branch is `overnight-fixes`, tree clean, no unpushed commits at start | done | `git status --short` empty, `git log origin/overnight-fixes..HEAD` empty, HEAD `f0b6340` |
| L02 | Bug 1: legacy empty skin colour accepted (419/475/486) | done | commit `c576e1d`; adapter skin tests |
| L03 | Bug 2: sub-chunk block batches expanded for 419 | done | commit `89bd30d`; protocol-419 expansion test |
| L04 | Bug 3: nested stack requests decoded in PlayerAuthInput (2168/2169) | done | commit `f0a02d3`; `TestPlayerAuthInputUsesProtocol2169BaseLayout`, compat sweep |
| L05 | Compat sweeps cover every advertised protocol | done | `internal/compat` runs the 15 adapter rows; protocol 2225 is gophertunnel's own native model, covered by its own pool and by `nativeUnwritable` in `sweep_test.go`. `go test ./internal/compat` passes (18.8 s, log `.fuzzlogs/fix2-test.log`) |
| L06 | Both conversion directions tested for every packet that differs from native | in-progress | `TestSampleRoundTrip` covers 54 realistic native packets clientbound; serverbound relies on per-adapter wire fixtures. Needs an inventory of differing packets and a check that each has a test |
| L07 | Every nested Marshal/Unmarshal uses the directional reader/writer | done | audited every `protocol.NewReader/NewWriter` and nested `Marshal` call site: sub-buffers in `items.go` build their own reader from a length-prefixed byte slice (correct), and the remaining bare `Marshal(io)` calls (`placeInContainerAction`) contain primitives only. All nested types with slices go through `io.directional()` (`stack_requests.go`, `shared_types.go`, `packets_basic.go`, `packets_v827.go`) |
| L08 | Every dropped or zeroed packet is justified from Mojang docs or implemented | done | drops are limited to: packet absent from the target pool (`outgoingPacketIDs` check in every 1.21+/1.26 adapter), `packetconv.UnsupportedNativePacket` (native-only IDs above the target era, plus boss events the target lacks), `legacyOnlyPacket766` in 748/766 (a 766-only packet), 419/486 `CraftingData` (built-in recipe clients), and the 944 target-only spawn egg; all are documented in the report's "Test matrix" and AGENTS.md |
| L09 | Legacy 419/475/486 parsers tolerate legal empty/optional values | done | audited every `InvalidValue` in the three legacy adapters: empty skin colour, empty store-offer UUID and empty descriptor name are accepted; the remaining rejections are malformed UUIDs, unknown items/blocks, negative counts and a non-zero behaviour-pack count the server cannot represent |
| L10 | Loopback harness runs at every protocol, including GameVersion variants in shared families (2168) | in-progress | `loopback_test.go` joins every advertised adapter through a real RakNet listener and dials 2168 twice (1.26.40 and 1.26.44); opt-in with `MV_LOOPBACK=1`; run pending after the fuzz sequence |
| L11 | Fuzz target for chunk/palette decoding | todo | only `FuzzAdapterDecode`/`FuzzConversionRoundTrip` exist |
| L12 | Fuzz target for login/negotiation | todo | same |
| L13 | Fuzz target for batch/compression | todo | same |
| L14 | Fuzz target for recipes/creative | todo | same |
| L15 | Fuzz target for nested-field packets | todo | same |
| L16 | Deep fuzz: 3 consecutive clean cycles per target, one target at a time | in-progress | first run found L30 and L32; cycles restarted after `0bd2472`. 10 minutes per cycle at `-parallel=1`, log `.fuzzlogs/deep.log` |
| L17 | Soak with many mixed-version clients clean under `-race` | todo | `TestSmoothnessSoak` is opt-in and not run under `-race` |
| L18 | Benchmarks exist for every converter | in-progress | `BenchmarkSmoothnessConversion`/`BenchmarkSmoothnessChunk` cover every adapter and packet family; `chunkconv/bench_test.go` covers sub-chunk/level chunk. Needs an explicit per-converter check |
| L19 | 2168 `SetScore` GameVersion-dependent layout exercised (was blocked) | in-progress | re-attempt: the seam `convertFromLatestForGameVersion` is unit-tested, and `ConvertFromLatest(..., nil)` covers the default; the `*minecraft.Conn` plumbing is the only uncovered line |
| L20 | Independent third-party client library replay (was blocked) | todo | re-attempt with a different approach: compare packet IDs and field layouts against PrismarineJS `minecraft-data` JSON instead of running a client |
| L21 | Real Mojang clients (was blocked) | blocked | cannot run in this environment; checklist in `docs/real-client-testing.md`; attempts logged in the report |
| L22 | Smoothness optimizations shipped with before/after numbers | done | commits `4f43945`, `00dfb9b`; report Performance table |
| L23 | Compression threshold and level tradeoff measured; defaults unchanged | done | `f0b6340`, report compression section |
| L24 | Chunk pacing analysed; no library change because the library has no send loop | done | report "Chunk send pacing" |
| L25 | CI configuration validates (actionlint-style check) | todo | `.github/workflows/ci.yml` and `codeql.yml` unvalidated this run |
| L26 | Independent reviewer round (`docs/completion-review.md`) | todo | not yet |
| L27 | Final report header: status, totals, top findings, real-client work | todo | `docs/overnight-bug-report.md` still carries the previous run's header |
| L28 | Final checks: build, vet, test ./..., race on touched packages and root, diff check, oracles | todo | last full run was the previous run's final commit `f0b6340` |
| L29 | Everything pushed to `origin/overnight-fixes` | todo | pushes happen per fix; final confirmation pending |
| L30 | Unbounded wire slice length caused a fatal 1.9 TB allocation (found by fuzzing) | done | commit `6303a27`; `TestOversizedWireLengthsRejected`; seed `internal/compat/testdata/fuzz/FuzzAdapterDecode/ec7faf06958d5620` reproduced `VirtualAlloc of 1954424520704 bytes failed` before the fix |
| L31 | Audit every raw `make([]T, wireCount)` in the adapters | done | `6303a27`: 21 slice-helper files plus the 1.18.10 `chunks.go`, 3 `skins.go`, 419 `items.go` and the 419/475/486 StartGame lists now call `protocol.FuncSliceOfLen`; `grep -rn 'make(\[\]T, count)' protocols` returns 0 |
| L32 | Second allocation class: texture pack / player list / skin / command counts (24 GB) | done | commit `0bd2472`; `packetio.CheckSliceLength` at every remaining site; `TestOversizedWireLengthsRejected` now covers SubChunk, PlayerList, ResourcePacksInfo and ResourcePackClientResponse |

## Decisions

- Fuzzing always uses `GOFLAGS="-tags="` because the machine's global tag breaks
  the default build.
- Long fuzz cycles run concurrently at two workers each; the report documents
  that higher parallelism exhausts memory on this machine.
- Legacy protocols stay behind `legacytransport`; the loopback harness covers
  them only when that tag and the transport fork are in use, and the default
  loopback covers the advertised families.
