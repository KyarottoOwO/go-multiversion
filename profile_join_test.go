//go:build mvprofile

package multiversion_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/pprof"
	"strconv"
	"testing"
	"time"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/login"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	multiversion "github.com/shawtymarco/go-multiversion"
	"github.com/shawtymarco/go-multiversion/chunkconv"
)

// This harness measures a real RakNet listener and a real join of a
// protocol-2193 client, instead of a loop over conversion functions. The client
// runs in a child process so the CPU profile captures only server-side work:
// otherwise the client's decrypting and unmarshalling in the same process would
// be averaged into the very numbers we attribute to the adapters.
//
//	go test -tags mvprofile -run TestProfileServerStartupAndJoin -v .
//	go tool pprof -top -nodecount=25 "$(go env TMPDIR)"/mv-profile-*/startup.cpu
//
// It is not part of the default suite: the tag keeps the loopback transport and
// the timer-dependent output out of normal verification.

const (
	profileClientAddrEnv = "MV_PROFILE_CLIENT_ADDR"
	profileChunkEnv      = "MV_PROFILE_CHUNK_COUNT"
	profileChunkCount    = 64
)

// TestProfileClientHelper is the external client. It is skipped unless the
// parent harness sets MV_PROFILE_CLIENT_ADDR.
func TestProfileClientHelper(t *testing.T) {
	address := os.Getenv(profileClientAddrEnv)
	if address == "" {
		t.Skip("loopback join client; set " + profileClientAddrEnv + " to run")
	}
	count := profileChunkCount
	if raw := os.Getenv(profileChunkEnv); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			t.Fatalf("chunk count: %v", err)
		}
		count = parsed
	}
	registry, items := perfFixture(t)
	proto, err := multiversion.V1_26_50WithRegistries(registry, items)
	if err != nil {
		t.Fatalf("client adapter: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	start := time.Now()
	dialer := minecraft.Dialer{
		Protocol:     proto,
		IdentityData: login.IdentityData{Identity: uuid.NewString(), DisplayName: "ProfileJoin"},
		ClientData:   login.ClientData{GameVersion: proto.Ver()},
	}
	client, err := dialer.DialContext(ctx, "raknet", address)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()
	dialed := time.Since(start)

	spawnStart := time.Now()
	if err := client.DoSpawnContext(ctx); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	spawned := time.Since(spawnStart)
	if err := client.WritePacket(&packet.Text{TextType: packet.TextTypeSystem, Message: "profile-client-ready"}); err != nil {
		t.Fatalf("ready: %v", err)
	}
	_ = client.SetReadDeadline(time.Now().Add(30 * time.Second))
	received := 0
	readStart := time.Now()
	for received < count {
		pk, err := client.ReadPacket()
		if err != nil {
			t.Fatalf("read %d/%d chunks: %v", received, count, err)
		}
		if _, ok := pk.(*packet.LevelChunk); ok {
			received++
		}
	}
	fmt.Printf("client: dial=%s spawn=%s read=%s (%d chunks) total=%s\n",
		dialed.Round(time.Microsecond), spawned.Round(time.Microsecond),
		time.Since(readStart).Round(time.Microsecond), count, time.Since(start).Round(time.Microsecond))
}

// TestProfileServerStartupAndJoin profiles catalogue construction plus listening
// (startup) and one complete join (StartGame, spawn and 64 real chunk sends).
func TestProfileServerStartupAndJoin(t *testing.T) {
	if os.Getenv(profileClientAddrEnv) != "" {
		t.Skip("client helper mode")
	}
	outDir := os.Getenv("MV_PROFILE_DIR")
	if outDir == "" {
		dir, err := os.MkdirTemp("", "mv-profile-")
		if err != nil {
			t.Fatalf("profile directory: %v", err)
		}
		outDir = dir
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("profile directory: %v", err)
	}

	stopStartup := startCPUProfile(t, filepath.Join(outDir, "startup.cpu"))
	fixtureStart := time.Now()
	registry, items := perfFixture(t)
	fixtureTook := time.Since(fixtureStart)
	catalogueStart := time.Now()
	adapters, err := (multiversion.Config{MinimumProtocol: 748}).ProtocolsWithRegistries(registry, items)
	if err != nil {
		t.Fatalf("catalogue: %v", err)
	}
	catalogueTook := time.Since(catalogueStart)
	listenStart := time.Now()
	listener, err := (minecraft.ListenConfig{AuthenticationDisabled: true, AcceptedProtocols: adapters}).Listen("raknet", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	listenTook := time.Since(listenStart)
	stopStartup()

	var adapter minecraft.Protocol
	for _, candidate := range adapters {
		if candidate.ID() == multiversion.V1_26_50().ID() {
			adapter = candidate
		}
	}
	if adapter == nil {
		t.Fatal("catalogue has no protocol-2193 adapter")
	}
	source := profileLevelChunk(t, registry, adapter, profileBiome(t, adapter))
	if _, err := chunkconv.Packet(source, adapter); err != nil {
		t.Fatalf("chunk fixture does not convert: %v", err)
	}
	count := profileChunkCount
	if raw := os.Getenv("MV_PROFILE_CHUNK_COUNT"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			t.Fatalf("chunk count: %v", err)
		}
		count = parsed
	}

	type joinResult struct {
		accept, startGame, spawn, chunks, convert, write, total time.Duration
		err                                                     error
	}
	stopJoin := startCPUProfile(t, filepath.Join(outDir, "join.cpu"))
	acceptStart := time.Now()
	results := make(chan joinResult, 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	go func() {
		var result joinResult
		result.total = time.Since(acceptStart)
		raw, err := listener.Accept()
		if err != nil {
			result.err = fmt.Errorf("accept: %w", err)
			results <- result
			return
		}
		result.accept = time.Since(acceptStart)
		conn, ok := raw.(*minecraft.Conn)
		if !ok {
			result.err = fmt.Errorf("accepted %T", raw)
			results <- result
			return
		}
		defer conn.Close()

		startGameStart := time.Now()
		err = conn.StartGameContext(ctx, minecraft.GameData{
			WorldName:                    "profile-join",
			BaseGameVersion:              "1.26.60",
			PlayerGameMode:               packet.GameTypeCreative,
			WorldGameMode:                packet.GameTypeCreative,
			PlayerPosition:               mgl32.Vec3{0, 64, 0},
			WorldSpawn:                   protocol.BlockPos{0, 64, 0},
			Items:                        items,
			GameRules:                    []protocol.GameRule{{Name: "doDaylightCycle", Value: false}},
			ServerAuthoritativeInventory: true,
			PlayerMovementSettings:       protocol.PlayerMovementSettings{ServerAuthoritativeBlockBreaking: true},
		})
		result.startGame = time.Since(startGameStart)
		if err != nil {
			result.err = fmt.Errorf("start game: %w", err)
			results <- result
			return
		}
		readyStart := time.Now()
		_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		for {
			pk, err := conn.ReadPacket()
			if err != nil {
				result.err = fmt.Errorf("wait for spawn: %w", err)
				results <- result
				return
			}
			if text, ok := pk.(*packet.Text); ok && text.Message == "profile-client-ready" {
				break
			}
		}
		result.spawn = time.Since(readyStart)

		chunkStart := time.Now()
		for range count {
			convertStart := time.Now()
			converted, err := chunkconv.Packet(source, adapter)
			result.convert += time.Since(convertStart)
			if err != nil {
				result.err = fmt.Errorf("convert chunk: %w", err)
				results <- result
				return
			}
			writeStart := time.Now()
			err = conn.WritePacket(converted)
			result.write += time.Since(writeStart)
			if err != nil {
				result.err = fmt.Errorf("write chunk: %w", err)
				results <- result
				return
			}
		}
		result.chunks = time.Since(chunkStart)
		result.total = time.Since(acceptStart)
		results <- result
	}()

	cmd := exec.Command(os.Args[0], "-test.run=^TestProfileClientHelper$")
	cmd.Env = append(os.Environ(),
		profileClientAddrEnv+"="+listener.Addr().String(),
		profileChunkEnv+"="+strconv.Itoa(count))
	clientOutput, clientErr := cmd.CombinedOutput()
	result := <-results
	stopJoin()

	t.Logf("startup: fixture=%s catalogue=%s listen=%s total=%s",
		fixtureTook.Round(time.Microsecond), catalogueTook.Round(time.Microsecond),
		listenTook.Round(time.Microsecond), (fixtureTook + catalogueTook + listenTook).Round(time.Microsecond))
	if clientErr != nil {
		t.Fatalf("client process: %v\n%s", clientErr, clientOutput)
	}
	t.Logf("%s", bytes.TrimSpace(clientOutput))
	if result.err != nil {
		t.Fatalf("server side: %v", result.err)
	}
	t.Logf("join: accept_incl_client_start=%s startgame=%s spawn_wait=%s send_%d_chunks=%s (convert=%s write=%s) server_total=%s",
		result.accept.Round(time.Microsecond), result.startGame.Round(time.Microsecond),
		result.spawn.Round(time.Microsecond), count,
		result.chunks.Round(time.Microsecond), result.convert.Round(time.Microsecond),
		result.write.Round(time.Microsecond), result.total.Round(time.Microsecond))
	t.Logf("profiles: %s", outDir)
}

func startCPUProfile(t *testing.T, path string) func() {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if err := pprof.StartCPUProfile(file); err != nil {
		t.Fatalf("start profile: %v", err)
	}
	return func() {
		pprof.StopCPUProfile()
		if err := file.Close(); err != nil {
			t.Errorf("close profile: %v", err)
		}
	}
}

// profileBiome returns a biome runtime ID the adapter can map, so the chunk
// fixture exercises the real biome conversion rather than failing on it.
func profileBiome(t *testing.T, adapter minecraft.Protocol) uint32 {
	t.Helper()
	mapper, ok := adapter.(interface {
		MapBiomeRuntimeID(uint32) (uint32, bool)
	})
	if !ok {
		return 195
	}
	for id := uint32(0); id < 4096; id++ {
		if _, mapped := mapper.MapBiomeRuntimeID(id); mapped {
			return id
		}
	}
	t.Fatal("adapter maps no biome runtime ID")
	return 0
}

// profileBlockIDs returns native runtime IDs for common states. They must be
// real registry entries: the adapter has to map them into the 2193 palette.
func profileBlockIDs(t *testing.T, registry effectsRegistry) []uint32 {
	t.Helper()
	wanted := []string{"minecraft:stone", "minecraft:dirt", "minecraft:grass_block", "minecraft:bedrock"}
	ids := make([]uint32, 0, len(wanted))
	for _, name := range wanted {
		found := false
		for id, state := range registry {
			if state.Name == name {
				ids = append(ids, uint32(id))
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("fixture has no %s", name)
		}
	}
	return ids
}

// profileLevelChunk builds a native overworld chunk: 24 sub-chunk sections and
// 24 biome sections, using the same version-9 layout the chunk tests round-trip.
func profileLevelChunk(t *testing.T, registry effectsRegistry, adapter minecraft.Protocol, biome uint32) *packet.LevelChunk {
	t.Helper()
	values := profileBlockIDs(t, registry)
	bits := 2
	section := append([]byte{9, 1, 0}, profilePalette(bits, values)...)
	// The sub-chunk tail is stripped here and written once for the chunk, so the
	// trim must account for exactly the bytes the fixture appended.
	section = append(section, 10, 0, 0, 8, 1, 2, 3, 0)
	section = section[:len(section)-8]
	out := new(bytes.Buffer)
	for range 24 {
		out.Write(section)
	}
	for i := range 24 {
		if i == 0 {
			out.Write(profilePalette(0, []uint32{biome}))
			continue
		}
		out.WriteByte(0xff)
	}
	out.Write([]byte{0, 10, 0, 42})
	return &packet.LevelChunk{
		Dimension:     packet.DimensionOverworld,
		SubChunkCount: 24,
		RawPayload:    out.Bytes(),
	}
}

// profilePalette writes the documented little-endian index words and zigzag
// palette values, mirroring the independent fixture in chunkconv's tests.
func profilePalette(bits int, values []uint32) []byte {
	w := new(bytes.Buffer)
	w.WriteByte(byte(bits<<1 | 1))
	if bits != 0 {
		words := make([]uint32, (4096+32/bits-1)/(32/bits))
		for cell := range 4096 {
			words[cell/(32/bits)] |= uint32(cell%len(values)) << uint(cell%(32/bits)*bits)
		}
		_ = binary.Write(w, binary.LittleEndian, words)
		_ = protocol.WriteVarint32(w, int32(len(values)))
	}
	for _, value := range values {
		_ = protocol.WriteVarint32(w, int32(value))
	}
	return w.Bytes()
}
