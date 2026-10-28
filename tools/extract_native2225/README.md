# Reproduce the .30 source acquisition

This is an explicit local extraction tool, not a CI startup hook. Use Linux/WSL,
Go 1.26 and Python 3. Its separate module pins published gophertunnel `4d6bffd`
(native 2225) without changing the library's production dependency.

Work in an ignored `tmp/native2225/` under the checkout. Download the official
Linux archive from the URL in `versions/1.26.60-preview.30-bds.json`, verify its
SHA256, then unpack it into `tmp/native2225/runtime/server`. `run.py` also verifies
the exact executable hash and refuses other worlds or a network with interfaces
other than loopback. Keep the archive, binaries, DB and logs out of Git.

From this tool directory (paths below are relative to it):

```sh
go build -o ../../tmp/native2225/capture ./capture
unshare -Urn python3 run.py vanilla \
  --server ../../tmp/native2225/runtime/server \
  --output ../../tmp/native2225/vanilla \
  --capture ../../tmp/native2225/capture
```

The first pass captures six allowlisted server packet bodies before any extraction
pack is added. Copy `palette-pack` to `tmp/native2225/palette-pack`; its item-name
input was generated from this exact build's captured ItemRegistry. Then enumerate
the live API registry and serialize the primary palette:

```sh
unshare -Urn python3 run.py palette \
  --server ../../tmp/native2225/runtime/server \
  --pack ../../tmp/native2225/palette-pack \
  --output ../../tmp/native2225/primary
go run ./candidates ../../data/native2223/raw/canonical_block_states.nbt \
  ../../tmp/native2225/runtime/server/worlds/native2225-extraction/db \
  ../../tmp/native2225/palette-pack/structures/native2225/probe.mcstructure
unshare -Urn python3 run.py palette \
  --server ../../tmp/native2225/runtime/server \
  --pack ../../tmp/native2225/palette-pack \
  --output ../../tmp/native2225/palette-output
```

The candidate command builds a probe, never the final palette. BDS must resolve
the 20 exceptional input states to native BlockPermutations and serialize the
result. The script defines no content and uses only a disposable extraction world.
The Script API's value arrays require `Array.from` in this build. State aliases
can normalize to the same permutation; duplicates and assignment failures are
recorded in the coverage report.

After the server has stopped, export through the read-only LevelDB decoder:

```sh
go run ./extract \
  -db ../../tmp/native2225/runtime/server/worlds/native2225-extraction/db \
  -capture ../../tmp/native2225/vanilla/capture \
  -log ../../tmp/native2225/palette-output/palette.log \
  -binary ../../tmp/native2225/runtime/server/bedrock_server \
  -archive ../../tmp/native2225/bedrock-server-1.26.60.30-linux.zip \
  -baseline ../../data/native2223/raw/canonical_block_states.nbt \
  -out ../../tmp/native2225/result
go test ./...
go vet ./...
```

The output directory must not exist. The exporter checks the exact binary and
archive, typed palette equality, uniqueness, packet round-trips, item IDs,
creative groups and block hashes, and script coverage. Failure leaves an
incomplete scratch output which must not be promoted. NBT map keys and palette
records are canonicalized; source list order and numeric tag types are preserved.
Promote only the validated output files and write the acquisition source manifest.
The root module's `data/native2225` tests independently check the retained assets.

References actually cloned and read locally:

- PMMP `bds-mod-mapping` at `368c45655421fda93f8bed55fe2f7844b1c21e14`:
  its `generate_palette` shows the native palette/serialized-NBT boundary. It is
  LGPL-3.0; no implementation was copied or linked into this independent tool.
  Its native offsets cannot be applied to this stripped .30 build.
- Microsoft `minecraft-scripting-samples` at
  `73a171fc8393a1052b4ca0669dc82231f775d8b1`: manifest/module setup reference.
- Official [BlockTypes](https://learn.microsoft.com/en-us/minecraft/creator/scriptapi/minecraft/server/blocktypes),
  [BlockStates](https://learn.microsoft.com/en-us/minecraft/creator/scriptapi/minecraft/server/blockstates),
  [StructureManager](https://learn.microsoft.com/en-us/minecraft/creator/scriptapi/minecraft/server/structuremanager)
  and [Structure](https://learn.microsoft.com/en-us/minecraft/creator/scriptapi/minecraft/server/structure)
  APIs define the enumeration and serializer used here.

This extraction does not enable .30 on a consumer or prove actual-client support.
