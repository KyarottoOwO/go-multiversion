package multiversion_test

import (
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	multiversion "github.com/shawtymarco/go-multiversion"
	"github.com/shawtymarco/go-multiversion/chunkconv"
	native "github.com/shawtymarco/go-multiversion/data/v2225"
)

// This file measures the cost the library itself controls: conversion latency,
// allocations, bytes put on the wire and chunk work per joining player. Client
// frame rate cannot be measured here and is deliberately never reported; the
// "Needs a real device" section of docs/overnight-bug-report.md explains how a
// human compares hitching on real hardware.

const (
	// smoothViewDistance is the client render distance used for the per-join
	// area: (2*10+1)^2 = 441 chunks, the vanilla default for a 10-chunk view.
	smoothViewDistance = 10
	// smoothLatencyBatches and smoothLatencyBatch size the percentile sample.
	// Timing a batch keeps the timer overhead well below one conversion.
	smoothLatencyBatches = 41
	smoothLatencyBatch   = 2048
)

var (
	smoothOnce    sync.Once
	smoothList    []minecraft.Protocol
	smoothChunks  map[int32]*packet.LevelChunk
	smoothProblem error
)

// smoothAdapters builds the full default catalogue once and prepares one
// realistic chunk fixture per protocol.
func smoothAdapters(tb testing.TB) []minecraft.Protocol {
	tb.Helper()
	smoothOnce.Do(func() {
		registry, items := perfFixture(tb)
		smoothList, smoothProblem = multiversion.ProtocolsWithRegistries(registry, items)
		if smoothProblem != nil {
			return
		}
		smoothChunks = make(map[int32]*packet.LevelChunk, len(smoothList))
		for _, p := range smoothList {
			chunk, err := smoothLevelChunk(tb, p)
			if err != nil {
				smoothProblem = fmt.Errorf("protocol %d chunk fixture: %w", p.ID(), err)
				return
			}
			smoothChunks[p.ID()] = chunk
		}
	})
	if smoothProblem != nil {
		tb.Fatal(smoothProblem)
	}
	return smoothList
}

func smoothRegistry(tb testing.TB) effectsRegistry {
	tb.Helper()
	registry, _ := perfFixture(tb)
	return registry
}

// smoothBlockIDs resolves a handful of common native states so the chunk
// palettes carry real registry IDs.
func smoothBlockIDs(tb testing.TB) []uint32 {
	tb.Helper()
	registry := smoothRegistry(tb)
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
			tb.Fatalf("registry has no %s", name)
		}
	}
	return ids
}

// smoothBiome returns the first native biome runtime ID the adapter can map.
func smoothBiome(tb testing.TB, p minecraft.Protocol) uint32 {
	tb.Helper()
	mapper, ok := p.(interface {
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
	tb.Fatalf("protocol %d maps no biome runtime ID", p.ID())
	return 0
}

// smoothLevelChunk builds a full overworld chunk in the version-9 network
// format: 24 sub-chunk sections with a four-entry block palette and 24 biome
// sections that reuse the first palette.
func smoothLevelChunk(tb testing.TB, p minecraft.Protocol) (*packet.LevelChunk, error) {
	tb.Helper()
	values := smoothBlockIDs(tb)
	section := append([]byte{9, 1, 0}, smoothPalette(2, values)...)
	out := new(bytes.Buffer)
	for range 24 {
		out.Write(section)
	}
	biome := smoothBiome(tb, p)
	for i := range 24 {
		if i == 0 {
			out.Write(smoothPalette(0, []uint32{biome}))
			continue
		}
		out.WriteByte(0xff)
	}
	out.Write([]byte{0, 10, 0, 42})
	chunk := &packet.LevelChunk{
		Position:      protocol.ChunkPos{1, 2},
		Dimension:     packet.DimensionOverworld,
		SubChunkCount: 24,
		RawPayload:    out.Bytes(),
	}
	if _, err := chunkconv.Packet(chunk, p); err != nil {
		return nil, err
	}
	return chunk, nil
}

// smoothPalette writes the version-9 palette layout: header byte, packed index
// words and zigzag palette values.
func smoothPalette(bits int, values []uint32) []byte {
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

// smoothPackets returns representative traffic for every direction. Packets a
// protocol does not carry are filtered out by the caller through its pool.
func smoothPackets(tb testing.TB) []struct {
	name        string
	clientbound bool
	pk          packet.Packet
} {
	tb.Helper()
	_, items := perfFixture(tb)
	entries := make(map[string]protocol.ItemEntry, len(items))
	for _, item := range items {
		entries[item.Name] = item
	}
	stone, ok := entries["minecraft:stone"]
	if !ok {
		tb.Fatal("fixture has no minecraft:stone item")
	}
	stack := protocol.ItemInstance{Stack: protocol.ItemStack{ItemType: protocol.ItemType{NetworkID: int32(stone.RuntimeID)}, Count: 1}}
	content := make([]protocol.ItemInstance, 36)
	for i := range content {
		content[i] = stack
	}
	return []struct {
		name        string
		clientbound bool
		pk          packet.Packet
	}{
		{"text", true, &packet.Text{TextType: packet.TextTypeChat, SourceName: "Steve", Message: "hello", XUID: "42", PlatformChatID: "pc"}},
		{"update_block", true, &packet.UpdateBlock{Position: protocol.BlockPos{1, 64, 3}, NewBlockRuntimeID: 1, Flags: packet.BlockUpdateNetwork}},
		{"move_player", true, &packet.MovePlayer{EntityRuntimeID: 3, Position: mgl32.Vec3{1, 64, 3}, Pitch: 10, Yaw: 20, Mode: packet.MoveModeReset}},
		{"set_actor_data", true, &packet.SetActorData{EntityRuntimeID: 9, EntityMetadata: protocol.EntityMetadata{protocol.EntityDataKeyName: "Zombie", protocol.EntityDataKeyFlags: int64(1)}}},
		{"level_sound", true, &packet.LevelSoundEvent{SoundType: packet.SoundEventPlace, Position: mgl32.Vec3{1, 64, 3}, ExtraData: 1, EntityType: "minecraft:player"}},
		{"inventory_content", true, &packet.InventoryContent{WindowID: 0, Content: content}},
		{"player_auth_input", false, &packet.PlayerAuthInput{Tick: 20, Position: mgl32.Vec3{1, 64, 3}, Pitch: 10, Yaw: 20}},
		{"inventory_transaction", false, &packet.InventoryTransaction{TransactionData: &protocol.NormalTransactionData{}, Actions: []protocol.InventoryAction{{
			SourceType: protocol.InventoryActionSourceContainer, InventorySlot: 1,
			OldItem: stack, NewItem: protocol.ItemInstance{},
		}}}},
	}
}

// convertSmooth runs one conversion in the requested direction, mirroring what
// a connection does for each packet it relays.
func convertSmooth(p minecraft.Protocol, pk packet.Packet, clientbound bool) []packet.Packet {
	if clientbound {
		return p.ConvertFromLatest(pk, nil)
	}
	return p.ConvertToLatest(pk, nil)
}

// smoothAvailable reports whether the protocol's pool carries the packet, so a
// benchmark never measures the unchanged pass-through of a foreign ID.
// gophertunnel's listener pool holds serverbound packets, so a clientbound
// packet lives in the opposite pool.
func smoothAvailable(p minecraft.Protocol, pk packet.Packet, clientbound bool) bool {
	_, ok := p.Packets(!clientbound)[pk.ID()]
	return ok
}

func percentiles(samples []time.Duration) (p50, p99, max time.Duration) {
	sorted := append([]time.Duration(nil), samples...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	p50 = sorted[len(sorted)/2]
	p99 = sorted[(len(sorted)*99)/100]
	return p50, p99, sorted[len(sorted)-1]
}

// TestSmoothnessLatency prints per-packet conversion latency percentiles for
// every protocol. Each sample times a batch, so timer overhead stays far below
// one conversion.
func TestSmoothnessLatency(t *testing.T) {
	if testing.Short() {
		t.Skip("latency profile is not part of -short")
	}
	packets := smoothPackets(t)
	t.Log("| protocol | packet | p50 | p99 | max |")
	t.Log("|---|---|---|---|---|")
	for _, p := range smoothAdapters(t) {
		for _, input := range packets {
			if !smoothAvailable(p, input.pk, input.clientbound) {
				continue
			}
			for range 50 {
				_ = convertSmooth(p, input.pk, input.clientbound)
			}
			samples := make([]time.Duration, smoothLatencyBatches)
			for i := range samples {
				start := time.Now()
				for range smoothLatencyBatch {
					_ = convertSmooth(p, input.pk, input.clientbound)
				}
				samples[i] = time.Since(start) / smoothLatencyBatch
			}
			p50, p99, max := percentiles(samples)
			t.Logf("| %d | %s | %s | %s | %s |", p.ID(), input.name, p50.Round(10*time.Nanosecond), p99.Round(10*time.Nanosecond), max.Round(10*time.Nanosecond))
		}
	}
}

// TestSmoothnessJoinCost prints what one joining player costs the server: the
// converted wire bytes for the login packets and a full view-distance area of
// chunks, plus the conversion time for that area.
func TestSmoothnessJoinCost(t *testing.T) {
	if testing.Short() {
		t.Skip("join cost profile is not part of -short")
	}
	blocks, err := native.BlockDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	_, items := perfFixture(t)
	startGame := &packet.StartGame{
		EntityUniqueID: 1, EntityRuntimeID: 2, PlayerGameMode: 1,
		PlayerPosition: mgl32.Vec3{0, 64, 0}, WorldSpawn: protocol.BlockPos{0, 64, 0},
		GameVersion: "1.26.60", BaseGameVersion: "1.26.60",
		LevelID: "level", WorldName: "world", Blocks: blocks, Time: 6000,
		PropertyData: map[string]any{},
	}
	itemRegistry := &packet.ItemRegistry{Items: items}
	chunks := (2*smoothViewDistance + 1) * (2*smoothViewDistance + 1)
	t.Log("| protocol | login bytes | packet bytes | chunk bytes | join bytes | 441 chunks | per chunk |")
	t.Log("|---|---|---|---|---|---|---|")
	for _, p := range smoothAdapters(t) {
		loginBytes := 0
		for _, pk := range []packet.Packet{startGame, itemRegistry} {
			for _, out := range p.ConvertFromLatest(pk, nil) {
				var buf bytes.Buffer
				out.Marshal(p.NewWriter(&buf, 0))
				loginBytes += buf.Len()
			}
		}
		chunk, ok := smoothChunks[p.ID()]
		if !ok {
			continue
		}
		var packetBytes, chunkBytes int
		start := time.Now()
		for range chunks {
			converted, err := chunkconv.Packet(chunk, p)
			if err != nil {
				t.Fatalf("protocol %d chunk: %v", p.ID(), err)
			}
			var buf bytes.Buffer
			converted.Marshal(p.NewWriter(&buf, 0))
			packetBytes += buf.Len()
			if level, ok := converted.(*packet.LevelChunk); ok {
				chunkBytes += len(level.RawPayload)
			}
		}
		took := time.Since(start)
		t.Logf("| %d | %d | %d | %d | %d | %s | %s |",
			p.ID(), loginBytes, packetBytes, chunkBytes, loginBytes+packetBytes,
			took.Round(time.Microsecond), (took / time.Duration(chunks)).Round(time.Microsecond))
	}
}

// TestSmoothnessScenes estimates the packet rate a scene can sustain. The work
// is the conversion only; socket writes and frame rate are out of scope.
func TestSmoothnessScenes(t *testing.T) {
	if testing.Short() {
		t.Skip("scene throughput profile is not part of -short")
	}
	t.Logf("catalogue: %d adapters; the native 2225 model is appended by gophertunnel per connection", len(smoothAdapters(t)))
	packets := smoothPackets(t)
	byName := make(map[string]packet.Packet, len(packets))
	for _, input := range packets {
		byName[input.name] = input.pk
	}
	// entity is one clientbound tick for one nearby entity.
	entity := func(id uint64) []packet.Packet {
		return []packet.Packet{
			&packet.MovePlayer{EntityRuntimeID: id, Position: mgl32.Vec3{1, 64, 3}, Pitch: 10, Yaw: 20, Mode: packet.MoveModeNormal},
			&packet.SetActorData{EntityRuntimeID: id, EntityMetadata: protocol.EntityMetadata{protocol.EntityDataKeyFlags: int64(1)}},
		}
	}
	scenes := []struct {
		name string
		pkts []packet.Packet
	}{
		{"movement", []packet.Packet{byName["player_auth_input"], byName["move_player"]}},
		{"building", []packet.Packet{byName["update_block"], byName["update_block"], byName["update_block"], byName["inventory_transaction"]}},
		{"entity_heavy", append(append(entity(1), entity(2)...), entity(3)...)},
	}
	t.Log("| protocol | movement/s | building/s | entity_heavy/s |")
	t.Log("|---|---|---|---|")
	for _, p := range smoothAdapters(t) {
		counts := make([]float64, len(scenes))
		for index, scene := range scenes {
			available := true
			for _, pk := range scene.pkts {
				_, clientbound := smoothDirection(pk)
				if !smoothAvailable(p, pk, clientbound) {
					available = false
					break
				}
			}
			if !available {
				continue
			}
			const budget = 20 * time.Millisecond
			iterations := 0
			start := time.Now()
			for time.Since(start) < budget {
				for _, pk := range scene.pkts {
					_, clientbound := smoothDirection(pk)
					_ = convertSmooth(p, pk, clientbound)
				}
				iterations++
			}
			counts[index] = float64(iterations) / time.Since(start).Seconds()
		}
		t.Logf("| %d | %.0f | %.0f | %.0f |", p.ID(), counts[0], counts[1], counts[2])
	}
}

// smoothDirection reports whether a fixture packet travels from the server.
func smoothDirection(pk packet.Packet) (name string, clientbound bool) {
	switch pk.(type) {
	case *packet.PlayerAuthInput, *packet.InventoryTransaction:
		return "serverbound", false
	}
	return "clientbound", true
}

// TestSmoothnessPayloadInflation checks that conversion does not grow a packet
// beyond what the target wire needs. A downgrade may legitimately add a fixed
// field or a palette, so the check reports the worst case per protocol instead
// of failing, and any large growth is called out in the report.
func TestSmoothnessPayloadInflation(t *testing.T) {
	if testing.Short() {
		t.Skip("payload inflation profile is not part of -short")
	}
	packets := smoothPackets(t)
	t.Log("| protocol | worst packet | native bytes | target bytes | growth |")
	t.Log("|---|---|---|---|---|")
	for _, p := range smoothAdapters(t) {
		worstName, worstNative, worstTarget := "", 0, 0
		worstGrowth := 0.0
		for _, input := range packets {
			if !input.clientbound || !smoothAvailable(p, input.pk, input.clientbound) {
				continue
			}
			var nativeWire bytes.Buffer
			input.pk.Marshal(protocol.NewWriter(&nativeWire, 0))
			for _, out := range p.ConvertFromLatest(input.pk, nil) {
				var target bytes.Buffer
				out.Marshal(p.NewWriter(&target, 0))
				growth := float64(target.Len()-nativeWire.Len()) / float64(max(nativeWire.Len(), 1))
				if growth > worstGrowth {
					worstName, worstNative, worstTarget = input.name, nativeWire.Len(), target.Len()
					worstGrowth = growth
				}
			}
		}
		if worstName == "" {
			continue
		}
		t.Logf("| %d | %s | %d | %d | %+.1f%% |", p.ID(), worstName, worstNative, worstTarget, worstGrowth*100)
	}
}

// TestSmoothnessCompressionTradeoff reports the CPU and bandwidth of the flate
// levels a chunk payload could use. It changes no default: the numbers only
// document what a server owner trades away.
func TestSmoothnessCompressionTradeoff(t *testing.T) {
	if testing.Short() {
		t.Skip("compression profile is not part of -short")
	}
	smoothAdapters(t)
	chunk := smoothChunks[2193]
	if chunk == nil {
		t.Fatal("no protocol-2193 chunk fixture")
	}
	// The benchmark chunk repeats four block states, which compresses far better
	// than real terrain. Compress a varied 8-bit section layout instead so the
	// tradeoff reflects a real chunk rather than a degenerate fixture.
	payload := smoothVariedChunkPayload()
	t.Log("| level | compressed bytes | ratio | time |")
	t.Log("|---|---|---|---|")
	for _, level := range []int{flate.BestSpeed, flate.DefaultCompression, flate.BestCompression} {
		var buffer bytes.Buffer
		writer, err := flate.NewWriter(&buffer, level)
		if err != nil {
			t.Fatal(err)
		}
		// Reset the writer so the measurement covers compression work rather
		// than the per-writer window allocation.
		const runs = 50
		start := time.Now()
		for range runs {
			buffer.Reset()
			writer.Reset(&buffer)
			if _, err := writer.Write(payload); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
		}
		took := time.Since(start) / runs
		t.Logf("| %d | %d | %.1f%% | %s |", level, buffer.Len(), float64(buffer.Len())*100/float64(len(payload)), took.Round(time.Microsecond))
	}
}

// smoothVariedChunkPayload builds one overworld chunk whose 8-bit palettes are
// filled from a linear congruential generator, approximating the entropy of
// real terrain.
func smoothVariedChunkPayload() []byte {
	out := new(bytes.Buffer)
	for range 24 {
		out.Write([]byte{9, 1, 0, 8<<1 | 1})
		words := make([]uint32, 4096/(32/8))
		seed := uint32(0x9e3779b9)
		for i := range words {
			var word uint32
			for cell := range 4 {
				seed = seed*1664525 + 1013904223
				word |= (seed >> 24) << uint(cell*8)
			}
			words[i] = word
		}
		_ = binary.Write(out, binary.LittleEndian, words)
		_ = protocol.WriteVarint32(out, 256)
		for i := range 256 {
			_ = protocol.WriteVarint32(out, int32(4000+i))
		}
	}
	for i := range 24 {
		if i == 0 {
			out.Write(smoothPalette(0, []uint32{195}))
			continue
		}
		out.WriteByte(0xff)
	}
	out.Write([]byte{0, 10, 0, 42})
	return out.Bytes()
}

// TestSmoothnessChunkGolden pins the exact bytes chunkconv produces for one
// full chunk on every protocol. Any optimization must keep this identical.
func TestSmoothnessChunkGolden(t *testing.T) {
	path := filepath.Join("testdata", "smoothness_chunk_goldens.txt")
	var current bytes.Buffer
	for _, p := range smoothAdapters(t) {
		chunk, ok := smoothChunks[p.ID()]
		if !ok {
			continue
		}
		converted, err := chunkconv.Packet(chunk, p)
		if err != nil {
			t.Fatalf("protocol %d: %v", p.ID(), err)
		}
		level, ok := converted.(*packet.LevelChunk)
		if !ok {
			t.Fatalf("protocol %d returned %T", p.ID(), converted)
		}
		sum := sha256.Sum256(level.RawPayload)
		fmt.Fprintf(&current, "%d %x %d\n", p.ID(), sum, len(level.RawPayload))
	}
	if os.Getenv("MV_UPDATE_SMOOTHNESS_GOLDENS") != "" {
		if err := os.WriteFile(path, current.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (regenerate with MV_UPDATE_SMOOTHNESS_GOLDENS=1): %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(want), bytes.TrimSpace(current.Bytes())) {
		t.Fatalf("chunk conversion bytes changed:\nwant:\n%s\ngot:\n%s", want, current.String())
	}
}

// BenchmarkSmoothnessConversion measures one packet conversion with allocation
// accounting, per protocol and packet.
func BenchmarkSmoothnessConversion(b *testing.B) {
	packets := smoothPackets(b)
	for _, p := range smoothAdapters(b) {
		for _, input := range packets {
			if !smoothAvailable(p, input.pk, input.clientbound) {
				continue
			}
			b.Run(fmt.Sprintf("%d/%s", p.ID(), input.name), func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					if len(convertSmooth(p, input.pk, input.clientbound)) == 0 {
						b.Fatal("conversion dropped the packet")
					}
				}
			})
		}
	}
}

// BenchmarkSmoothnessChunk measures one chunk translation per protocol.
func BenchmarkSmoothnessChunk(b *testing.B) {
	for _, p := range smoothAdapters(b) {
		chunk, ok := smoothChunks[p.ID()]
		if !ok {
			continue
		}
		b.Run(strconv.Itoa(int(p.ID())), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(chunk.RawPayload)))
			b.ResetTimer()
			for range b.N {
				if _, err := chunkconv.Packet(chunk, p); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// TestSmoothnessSoak runs mixed-version clients against the catalogue for a
// fixed duration and reports heap growth, GC pauses, goroutines and lock
// contention. It is opt-in because it is a measurement, not a correctness
// check: MV_SMOOTHNESS_SOAK=<seconds>.
func TestSmoothnessSoak(t *testing.T) {
	raw := os.Getenv("MV_SMOOTHNESS_SOAK")
	if raw == "" {
		t.Skip("set MV_SMOOTHNESS_SOAK=<seconds> to run the soak")
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		t.Fatalf("invalid MV_SMOOTHNESS_SOAK %q", raw)
	}
	adapters := smoothAdapters(t)
	packets := smoothPackets(t)
	if len(adapters) == 0 {
		t.Fatal("no adapters")
	}
	registry, items := perfFixture(t)
	if _, err := multiversion.ProtocolsWithRegistries(registry, items); err != nil {
		t.Fatal(err)
	}
	workers := 32
	if raw := os.Getenv("MV_SMOOTHNESS_WORKERS"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			workers = parsed
		}
	}

	runtime.SetMutexProfileFraction(1)
	runtime.SetBlockProfileRate(1)
	defer func() {
		runtime.SetMutexProfileFraction(0)
		runtime.SetBlockProfileRate(0)
	}()

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	goroutinesBefore := runtime.NumGoroutine()

	type job struct {
		pk          packet.Packet
		clientbound bool
	}
	deadline := time.Now().Add(time.Duration(seconds) * time.Second)
	var conversions int64
	counts := make([]int64, workers)
	var wg sync.WaitGroup
	for worker := range workers {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			p := adapters[worker%len(adapters)]
			chunk := smoothChunks[p.ID()]
			// Resolve availability once: pool construction is a table build, not
			// relay work, and would otherwise dominate this measurement.
			jobs := make([]job, 0, len(packets))
			for _, input := range packets {
				if smoothAvailable(p, input.pk, input.clientbound) {
					jobs = append(jobs, job{input.pk, input.clientbound})
				}
			}
			local := int64(0)
			for time.Now().Before(deadline) {
				for _, current := range jobs {
					_ = convertSmooth(p, current.pk, current.clientbound)
				}
				if chunk != nil {
					_, _ = chunkconv.Packet(chunk, p)
				}
				local++
			}
			// A per-worker counter avoids the contention this test measures.
			counts[worker] = local
		}(worker)
	}
	wg.Wait()
	for _, count := range counts {
		conversions += count
	}

	goroutinesAfter := runtime.NumGoroutine()
	// Force a collection so the reported heap is the steady state rather than
	// the last round's uncollected garbage.
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	pauses := after.PauseTotalNs - before.PauseTotalNs
	longest := uint64(0)
	for _, pause := range after.PauseNs {
		if pause > longest {
			longest = pause
		}
	}
	t.Logf("soak: %d workers, %d protocols, %d rounds (%.0f rounds/s)", workers, len(adapters), conversions, float64(conversions)/float64(seconds))
	t.Logf("heap: live %d -> %d bytes, total allocated %d bytes (%.1f MB/s), sys %d -> %d",
		before.HeapAlloc, after.HeapAlloc, after.TotalAlloc-before.TotalAlloc,
		float64(after.TotalAlloc-before.TotalAlloc)/float64(seconds)/1e6, before.Sys, after.Sys)
	t.Logf("gc: %d -> %d cycles, pause total %s, longest pause %s, goroutines %d -> %d",
		before.NumGC, after.NumGC, time.Duration(pauses), time.Duration(longest), goroutinesBefore, goroutinesAfter)

	dir, err := os.MkdirTemp("", "mv-soak-")
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"mutex", "block", "heap"} {
		file, err := os.Create(filepath.Join(dir, kind+".pprof"))
		if err != nil {
			t.Fatal(err)
		}
		if err := pprof.Lookup(kind).WriteTo(file, 0); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("profiles: %s", dir)
	if goroutinesAfter > goroutinesBefore+workers/4 {
		t.Errorf("goroutines grew from %d to %d", goroutinesBefore, goroutinesAfter)
	}
}
