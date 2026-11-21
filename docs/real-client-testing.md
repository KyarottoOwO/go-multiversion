# Real-client testing checklist

The automated suites in this repository prove wire layouts, conversion paths and
round trips against frozen historical data. They cannot prove that a retail
Minecraft client draws, plays or behaves correctly, because no Mojang client runs
in CI. Use this checklist before claiming a protocol family works with real
players.

Every item below was **not** executed by automation. Where automation covers part
of an item, it is named so a tester knows what is already known to be correct.

## Per-version checklist

Run this list for every protocol you intend to ship:

| Protocol | Client version(s) |
|----------|-------------------|
| 2225 | 1.26.60 (native baseline) |
| 2193 | 1.26.50 – 1.26.52 |
| 2169 | 1.26.45 |
| 2168 | 1.26.44, and 1.26.40 – 1.26.43 |
| 1001 | 1.26.20, 1.26.21, 1.26.23 (and the 1.26.30 – 1.26.36 builds) |
| 975 | 1.26.20 – 1.26.23 |
| 944 | 1.26.10 – 1.26.14 |
| 924 | 1.26.0 – 1.26.3 |
| 898 | 1.21.130 – 1.21.132 |
| 844 | 1.21.110 – 1.21.114 |
| 827 | 1.21.100 – 1.21.102 |
| 766 | 1.21.50 – 1.21.51 |
| 748 | 1.21.40, 1.21.41, 1.21.43, 1.21.44 |
| 486 | 1.18.10 – 1.18.12 (opt-in `legacytransport` build) |
| 475 | 1.18.0 – 1.18.2 (opt-in `legacytransport` build) |
| 419 | 1.16.100 (opt-in `legacytransport` build) |

For each protocol, check:

1. **Join and login.** Fresh install with no cached packs. Encryption on and off
   (`online-mode` style vs offline). Repeat with each compression setting the
   server exposes.
2. **Resource pack negotiation.** Accept and decline. Decline must still let the
   client join, and packs must not restart in a loop.
3. **Movement.** Walk, sprint, jump, sneak, swim, fly (creative), and climb a
   ladder. Movement, camera and input flags are the most version-sensitive part
   of the protocol; watch for rubber-banding and stuck keys.
4. **Chunk loading.** Render distance 4, 12 and the server maximum. Walk across a
   chunk border, teleport across dimensions, and reload the world. Watch for
   missing sections, holes, wrong biomes, invisible blocks at the top and bottom
   of the world, and empty chunks after respawn.
5. **Block families.** Place and break one block from each family: stone,
   dirt/grass, wood planks and logs, wool (all colours), glass, stairs, slabs,
   fences, doors, trapdoors, buttons, pressure plates, torches, water and lava
   (both layers of waterlogging), containers (chest, barrel, furnace), and a
   block with a block entity (sign, banner, bed, skull).
6. **Inventory.** Pick up, split, merge, quick-move, drop, swap hotbar slots,
   drag across slots, and use the creative menu (search, tabs, destroy item).
7. **Item stack requests.** Click rapidly in the inventory. A client bundles these
   as stack requests inside PlayerAuthInput on 2168+; a click that does nothing,
   or a disconnect right after clicking, is a failure.
8. **Entities.** Spawn a mob, a dropped item, a boat, and another player. Check
   nametags, skins (including a player with no skin set), equipment, animations,
   and that metadata such as health and custom names apply.
9. **Chat and UI.** Chat, `/commands`, forms (if the server has them), scoreboard,
   boss bar, title/subtitle and action bar.
10. **Books.** Write in a book and quill, sign it, and read it back.
11. **Sounds.** Step sounds, block break sounds, music discs, and a door opening.
12. **Disconnect and reconnect.** Kick, quit, close the game, and rejoin quickly
    several times. No hang, no "outdated client" false positive.
13. **Multiple clients.** Two clients on different protocols at once, seeing and
    interacting with each other (blocks, entities, chat).
14. **Unlisted protocol.** A client build whose protocol the server does not list
    must receive a clean, logged disconnect, never a hang or a crash.

## Platforms and input modes to cover

- Windows (mouse and keyboard), Android (touch), iOS/iPadOS (touch), Xbox
  (controller), PlayStation (controller), Nintendo Switch (controller).
- Keyboard/mouse and controller clients send different auth-input bit layouts, and
  touch clients send extra input flags.
- Tablet vs phone layout matters for inventory UI on touch clients.

## Bug report template

Copy this into the issue tracker for every failure:

```
Client version:
Platform / input mode:
Protocol ID (from the server log line "client connected", or /version):
Server version and build (commit):
Steps to reproduce:
  1.
  2.
  3.
Expected:
Actual:
Server log lines (include the packet ID and protocol if present):
Packet capture (if available; note whether it was taken with a proxy):
Repeatable? (always / sometimes / once):
```

## What the automated suites already cover

| Area | Covered by automation | Still needs a client |
|------|----------------------|----------------------|
| Packet layouts and field order | frozen per-version wire oracles (`tools/oracle_v*`, adapter `testdata`), `internal/compat` sample round trip | rendering and gameplay effects |
| Conversion both ways, no input mutation | `TestSampleRoundTrip`, adapter unit tests | — |
| Unknown/absent values (sounds, blocks, items) | adapter filters, fallback statistics test data | visual result of fallbacks |
| Truncated and malformed input | `TestDecoderTruncationSweep`, `FuzzAdapterDecode` | — |
| Hostile decoded values through conversions | `FuzzConversionRoundTrip` | — |
| Chunk palettes (version 9 and pre-hash) | `chunkconv` tests, adapter palette tests | what the client actually draws |
| Start game, creative content, recipes | pool round trips and sample harness | recipe book and creative menu behaviour |
| Skins | legacy skin tests (including empty colours) | how the skin renders |
| Login timeouts, encryption, compression | `minecraft.Listener` tests in the upstream fork | real handshake timing |
| Goroutine leaks on disconnect | `go test -race ./...` | long sessions |
| Block changes (sub-chunk batches) | protocol-419 expansion test | that the client applies them without a chunk reload |

Commit results to the report when a client check passes, and name the exact
client build and platform. A protocol that only passes automated checks stays
"unverified with a real client" in the report.
