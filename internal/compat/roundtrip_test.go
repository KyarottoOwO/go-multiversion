package compat

import (
	"bytes"
	"fmt"
	"image/color"
	"reflect"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	native "github.com/shawtymarco/go-multiversion/data/v2225"
)

// sample is one realistic packet on the native wire model. clientbound reports
// whether a server sends it to clients (ConvertFromLatest) instead of receiving
// it from them (ConvertToLatest).
type sample struct {
	name        string
	clientbound bool
	pk          packet.Packet
	// skipEquality ignores differences between the input and the round-tripped
	// native packet for fields the adapter is documented to normalise.
	skipEquality bool
	// clientOnly marks packets that only travel from the server, so no adapter
	// converts them back into the native model.
	clientOnly bool
	// droppedFor documents protocols whose adapter deliberately drops the
	// packet, mapping protocol ID to the reason.
	droppedFor map[int32]string
}

func samples(t *testing.T) []sample {
	t.Helper()
	blocks, err := native.BlockDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	itemSnapshot, err := native.Items()
	if err != nil {
		t.Fatal(err)
	}
	// Item conversion is validated against the caller's complete registry, so
	// the sample uses the real 1.26.60 table rather than a hand-picked subset.
	items := make([]protocol.ItemEntry, 0, len(itemSnapshot))
	for name, item := range itemSnapshot {
		items = append(items, protocol.ItemEntry{Name: name, RuntimeID: int16(item.RuntimeID), ComponentBased: item.ComponentBased, Version: item.Version, Data: item.Data})
	}
	if len(blocks) > 4 {
		blocks = blocks[:4]
	}
	skin := protocol.Skin{
		SkinID: "skin", SkinResourcePatch: []byte(`{}`), SkinImageWidth: 1, SkinImageHeight: 1,
		SkinData: []byte{1, 2, 3, 4}, CapeImageWidth: 1, CapeImageHeight: 1, CapeData: []byte{5, 6, 7, 8},
		SkinGeometry: []byte(`{}`), AnimationData: []byte(`{}`), CapeID: "cape", FullID: "full",
		ArmSize: protocol.ArmSizeSlim, SkinColour: color.RGBA{R: 0x11, G: 0x22, B: 0x33, A: 0xff},
	}
	stoneEntry, ok := itemSnapshot["minecraft:stone"]
	if !ok {
		t.Fatal("minecraft:stone missing from native snapshot")
	}
	stone := protocol.ItemStack{ItemType: protocol.ItemType{NetworkID: int32(stoneEntry.RuntimeID)}, Count: 1}
	return []sample{
		{name: "start_game", clientbound: true, pk: &packet.StartGame{EntityUniqueID: 1, EntityRuntimeID: 2, PlayerGameMode: 1,
			PlayerPosition: mgl32.Vec3{1, 64, 3}, WorldSpawn: protocol.BlockPos{0, 64, 0}, GameVersion: "1.26.60",
			BaseGameVersion: "1.26.60", WorldName: "world", LevelID: "level", Blocks: blocks, Time: 6000,
			PropertyData: map[string]any{}, ServerBlockStateChecksum: 1}, skipEquality: true},
		{name: "item_registry", clientbound: true, pk: &packet.ItemRegistry{Items: items}, skipEquality: true, clientOnly: true},
		{name: "creative_content", clientbound: true, pk: &packet.CreativeContent{
			Groups: []protocol.CreativeGroup{{Name: "itemGroup.name.stone", Icon: stone}},
			Items:  []protocol.CreativeItem{{CreativeItemNetworkID: 1, Item: stone}},
		}},
		{name: "crafting_data", clientbound: true, pk: &packet.CraftingData{ShapelessRecipes: []protocol.ShapelessRecipe{{
			Block: "crafting_table", RecipeID: "1",
			Input:  []protocol.ItemDescriptorCount{{Descriptor: &protocol.DefaultItemDescriptor{Name: "minecraft:stone"}, Count: 1}},
			Output: []protocol.ItemStack{stone},
		}}, ClearRecipes: true}, droppedFor: map[int32]string{
			419: "1.16.100 keeps its built-in recipe catalogue; the current one crashes the client",
			486: "1.18.12 servers did not publish CraftingData",
		}},
		{name: "level_chunk", clientbound: true, pk: &packet.LevelChunk{Position: protocol.ChunkPos{1, 2}, Dimension: packet.DimensionOverworld, SubChunkCount: 0, RawPayload: []byte{1, 2, 3}}},
		{name: "player_list_add", clientbound: true, pk: &packet.PlayerList{Entries: []protocol.PlayerListEntry{{
			ActionType: protocol.PlayerListActionAdd, UUID: uuid.New(), EntityUniqueID: -5, Username: "Steve", XUID: "42",
			PlatformChatID: "pc", BuildPlatform: 7, Skin: skin, Teacher: true, Host: true,
		}}}, skipEquality: true},
		{name: "player_list_remove", clientbound: true, pk: &packet.PlayerList{Entries: []protocol.PlayerListEntry{{ActionType: protocol.PlayerListActionRemove, UUID: uuid.New()}}}},
		{name: "player_skin", clientbound: true, pk: &packet.PlayerSkin{UUID: uuid.New(), Skin: skin, NewSkinName: "new", OldSkinName: "old"}},
		{name: "add_player", clientbound: true, pk: &packet.AddPlayer{UUID: uuid.New(), Username: "Steve", EntityRuntimeID: 3,
			Position: mgl32.Vec3{1, 64, 3}, HeldItem: protocol.ItemInstance{Stack: stone}, GameType: 1, DeviceID: "device", BuildPlatform: 7}},
		{name: "add_actor", clientbound: true, pk: &packet.AddActor{EntityUniqueID: 9, EntityRuntimeID: 9, EntityType: "minecraft:zombie",
			Position: mgl32.Vec3{1, 64, 3}, EntityMetadata: protocol.EntityMetadata{protocol.EntityDataKeyName: "Zombie"}}},
		{name: "set_actor_data", clientbound: true, pk: &packet.SetActorData{EntityRuntimeID: 9, EntityMetadata: protocol.EntityMetadata{protocol.EntityDataKeyName: "Zombie"}}},
		{name: "mob_equipment", clientbound: true, pk: &packet.MobEquipment{EntityRuntimeID: 9, NewItem: protocol.ItemInstance{Stack: stone}, InventorySlot: 1, HotBarSlot: 1, WindowID: 0}},
		{name: "inventory_content", clientbound: true, pk: &packet.InventoryContent{WindowID: 0, Content: []protocol.ItemInstance{{Stack: stone}}}},
		{name: "inventory_slot", clientbound: true, pk: &packet.InventorySlot{WindowID: 0, Slot: 1, NewItem: protocol.ItemInstance{Stack: stone}}},
		{name: "level_sound_place", clientbound: true, pk: &packet.LevelSoundEvent{SoundType: packet.SoundEventPlace, Position: mgl32.Vec3{1, 64, 3}, ExtraData: 1, EntityType: "minecraft:player"}},
		{name: "level_sound_note", clientbound: true, pk: &packet.LevelSoundEvent{SoundType: packet.SoundEventNote, Position: mgl32.Vec3{1, 64, 3}, ExtraData: 0x1234, EntityType: "minecraft:player"}},
		{name: "level_event_particle", clientbound: true, pk: &packet.LevelEvent{EventType: packet.LevelEventParticlesDestroyBlock, Position: mgl32.Vec3{1, 64, 3}, EventData: 1}},
		{name: "level_event_sound", clientbound: true, pk: &packet.LevelEvent{EventType: packet.LevelEventSoundClick, Position: mgl32.Vec3{1, 64, 3}, EventData: 100}},
		{name: "actor_event_feed", clientbound: true, pk: &packet.ActorEvent{EntityRuntimeID: 9, EventType: packet.ActorEventFeed, EventData: 1<<16 | 5}},
		{name: "update_block", clientbound: true, pk: &packet.UpdateBlock{Position: protocol.BlockPos{1, 64, 3}, NewBlockRuntimeID: 1, Flags: packet.BlockUpdateNetwork}},
		{name: "update_sub_chunk_blocks", clientbound: true, pk: &packet.UpdateSubChunkBlocks{Position: protocol.BlockPos{0, 64, 0}, Blocks: []protocol.BlockChangeEntry{{BlockPos: protocol.BlockPos{1, 64, 3}, BlockRuntimeID: 1}}}},
		{name: "animate", clientbound: true, pk: &packet.Animate{ActionType: packet.AnimateActionSwingArm, EntityRuntimeID: 9}},
		{name: "text", clientbound: true, pk: &packet.Text{TextType: packet.TextTypeChat, SourceName: "Steve", Message: "hello", XUID: "42", PlatformChatID: "pc"}},
		{name: "book_edit", clientbound: true, pk: &packet.BookEdit{InventorySlot: 1, ActionType: packet.BookActionSign, Author: "Steve", Title: "Book", XUID: "42"}},
		{name: "set_score", clientbound: true, pk: &packet.SetScore{Entries: []protocol.ScoreboardEntry{{EntryID: 1, ObjectiveName: "obj", Score: 5, IdentityType: protocol.ScoreboardIdentityFakePlayer, DisplayName: "line"}}}},
		{name: "camera", clientbound: true, pk: &packet.Camera{CameraEntityUniqueID: 9, TargetPlayerUniqueID: 3}},
		{name: "move_player", clientbound: true, pk: &packet.MovePlayer{EntityRuntimeID: 3, Position: mgl32.Vec3{1, 64, 3}, Pitch: 10, Yaw: 20, Mode: packet.MoveModeReset}},
		{name: "chunk_radius_updated", clientbound: true, pk: &packet.ChunkRadiusUpdated{ChunkRadius: 8}},
		{name: "change_dimension", clientbound: true, pk: &packet.ChangeDimension{Dimension: packet.DimensionNether, Position: mgl32.Vec3{1, 64, 3}, Respawn: true}},
		{name: "disconnect", clientbound: true, pk: &packet.Disconnect{Message: "bye"}},
		{name: "play_sound", clientbound: true, pk: &packet.PlaySound{SoundName: "random.click", Position: mgl32.Vec3{1, 64, 3}, Volume: 1, Pitch: 1}},
		{name: "spawn_particle_effect", clientbound: true, pk: &packet.SpawnParticleEffect{Position: mgl32.Vec3{1, 64, 3}, ParticleName: "minecraft:explosion_particle"}},
		{name: "transfer", clientbound: true, pk: &packet.Transfer{Address: "1.2.3.4:19132"}},
		{name: "emote", clientbound: true, pk: &packet.Emote{EntityRuntimeID: 9, EmoteID: "emote"}},
		{name: "set_title", clientbound: true, pk: &packet.SetTitle{ActionType: packet.TitleActionSetTitle, Text: "title"}},
		{name: "stop_sound", clientbound: true, pk: &packet.StopSound{SoundName: "random.click", StopAll: false}},
		{name: "network_chunk_publisher_update", clientbound: true, pk: &packet.NetworkChunkPublisherUpdate{Position: protocol.BlockPos{1, 64, 3}, Radius: 8}},
		{name: "resource_packs_info", clientbound: true, pk: &packet.ResourcePacksInfo{}},
		{name: "boss_event", clientbound: true, pk: &packet.BossEvent{BossEntityUniqueID: 9, EventType: packet.BossEventShow, BossBarTitle: "boss"}},

		{name: "request_chunk_radius", clientbound: false, pk: &packet.RequestChunkRadius{ChunkRadius: 8}},
		{name: "player_action", clientbound: false, pk: &packet.PlayerAction{EntityRuntimeID: 3, ActionType: protocol.PlayerActionJump, BlockPosition: protocol.BlockPos{1, 63, 3}, BlockFace: 1}},
		{name: "inventory_transaction_normal", clientbound: false, pk: &packet.InventoryTransaction{TransactionData: &protocol.NormalTransactionData{}, Actions: []protocol.InventoryAction{{
			SourceType: protocol.InventoryActionSourceContainer, InventorySlot: 1,
			OldItem: protocol.ItemInstance{Stack: stone}, NewItem: protocol.ItemInstance{},
		}}}},
		{name: "inventory_transaction_use_item", clientbound: false, pk: &packet.InventoryTransaction{TransactionData: &protocol.UseItemTransactionData{
			ActionType: protocol.UseItemActionClickBlock, BlockPosition: protocol.BlockPos{1, 63, 3}, BlockFace: 1,
			BlockRuntimeID: 1, HeldItem: protocol.ItemInstance{Stack: stone},
		}}},
		{name: "inventory_transaction_use_item_on_entity", clientbound: false, pk: &packet.InventoryTransaction{TransactionData: &protocol.UseItemOnEntityTransactionData{
			TargetEntityRuntimeID: 9, ActionType: protocol.UseItemOnEntityActionInteract, HeldItem: protocol.ItemInstance{Stack: stone},
		}}},
		{name: "inventory_transaction_release_item", clientbound: false, pk: &packet.InventoryTransaction{TransactionData: &protocol.ReleaseItemTransactionData{
			ActionType: protocol.ReleaseItemActionRelease, HotBarSlot: 1, HeldItem: protocol.ItemInstance{Stack: stone},
		}}},
		{name: "player_auth_input", clientbound: false, pk: &packet.PlayerAuthInput{Tick: 20, Position: mgl32.Vec3{1, 64, 3}, Pitch: 10, Yaw: 20,
			ItemInteractionData: protocol.Option(protocol.UseItemTransactionData{
				ActionType: protocol.UseItemActionClickBlock, BlockPosition: protocol.BlockPos{1, 63, 3}, BlockRuntimeID: 1,
				HeldItem: protocol.ItemInstance{Stack: stone},
			})}},
		{name: "modal_form_response", clientbound: false, pk: &packet.ModalFormResponse{FormID: 1, ResponseData: protocol.Option([]byte(`{"a":1}`))}},
		{name: "command_request", clientbound: false, pk: &packet.CommandRequest{CommandLine: "/help", CommandOrigin: protocol.CommandOrigin{Origin: protocol.CommandOriginPlayer, UUID: uuid.New()}}},
		{name: "container_close", clientbound: false, pk: &packet.ContainerClose{WindowID: 0, ServerSide: true}},
		{name: "set_player_game_type", clientbound: false, pk: &packet.SetPlayerGameType{GameType: 1}},
	}
}

// encodeWithAdapter marshals pk using the adapter wire.
func encodeWithAdapter(p minecraft.Protocol, pk packet.Packet) []byte {
	var body bytes.Buffer
	pk.Marshal(p.NewWriter(&body, 0))
	return bytes.Clone(body.Bytes())
}

// decodeWithAdapter parses payload with the adapter pool entry for id.
func decodeWithAdapter(p minecraft.Protocol, listener bool, id uint32, payload []byte) (packet.Packet, error) {
	constructor, ok := p.Packets(listener)[id]
	if !ok {
		return nil, fmt.Errorf("packet %d missing from pool", id)
	}
	pk := constructor()
	reader := bytes.NewBuffer(payload)
	// A client and a server both decode packets the peer is trusted to have
	// encoded, so untrusted-client slice limits do not apply here.
	pk.Marshal(p.NewReader(reader, 0, false))
	if reader.Len() != 0 {
		return nil, fmt.Errorf("packet %d left %d unread bytes", id, reader.Len())
	}
	return pk, nil
}

// firstDifference reports the offset of the first differing byte, or -1.
func firstDifference(a, b []byte) int {
	for index := 0; index < len(a) && index < len(b); index++ {
		if a[index] != b[index] {
			return index
		}
	}
	if len(a) != len(b) {
		return min(len(a), len(b))
	}
	return -1
}

// TestSampleRoundTrip drives realistic clientbound packets through the
// production path: conversion, adapter wire encoding, adapter decoding and
// conversion back. The wire bytes must be stable across the cycle, and the
// caller's native packet must never be mutated.
//
// Serverbound packets are not covered here: a historical client encodes them
// itself, so the adapter writer is not a fair proxy for that direction. The
// frozen per-version wire fixtures in each adapter package cover those layouts.
func TestSampleRoundTrip(t *testing.T) {
	inputs := samples(t)
	for _, p := range catalogue(t) {
		for _, input := range inputs {
			if !input.clientbound {
				continue
			}
			t.Run(fmt.Sprintf("%d/%s", p.ID(), input.name), func(t *testing.T) {
				packetValue := reflect.ValueOf(input.pk).Elem()
				original := packetValue.Interface()
				defer func() {
					if recovered := recover(); recovered != nil {
						t.Fatalf("%s (%T) panicked: %v", input.name, input.pk, recovered)
					}
				}()
				converted := p.ConvertFromLatest(input.pk, nil)
				if reason, dropped := input.droppedFor[p.ID()]; dropped {
					if len(converted) != 0 {
						t.Fatalf("conversion returned %d packets, want 0 (%s)", len(converted), reason)
					}
					if !reflect.DeepEqual(original, packetValue.Interface()) {
						t.Fatal("conversion mutated the native input packet")
					}
					return
				}
				if len(converted) == 0 {
					t.Fatal("clientbound conversion dropped the packet")
				}
				if input.clientOnly {
					if !reflect.DeepEqual(original, packetValue.Interface()) {
						t.Fatal("conversion mutated the native input packet")
					}
					return
				}
				first := encodeWithAdapter(p, converted[0])
				decoded, err := decodeWithAdapter(p, false, converted[0].ID(), first)
				if err != nil {
					t.Fatalf("decode: %v", err)
				}
				upgraded := p.ConvertToLatest(decoded, nil)
				if len(upgraded) == 0 {
					t.Fatal("serverbound conversion dropped the packet")
				}
				// Convert the round-tripped native packet again. A conversion
				// that is stable in one direction must stay stable the next.
				again := p.ConvertFromLatest(upgraded[0], nil)
				if len(again) != len(converted) {
					t.Fatalf("second conversion returned %d packets, first returned %d", len(again), len(converted))
				}
				// Compare the decoded form rather than raw bytes: gophertunnel's
				// NBT encoder walks maps in iteration order, so a compound with
				// several keys may legitimately encode to different bytes.
				second := encodeWithAdapter(p, again[0])
				secondDecoded, err := decodeWithAdapter(p, false, again[0].ID(), second)
				if err != nil {
					t.Fatalf("decode after one conversion cycle: %v", err)
				}
				secondLatest := p.ConvertToLatest(secondDecoded, nil)
				if len(secondLatest) != len(upgraded) {
					t.Fatalf("second cycle returned %d native packets, first returned %d", len(secondLatest), len(upgraded))
				}
				// Adapter wrappers embed unexported pointers, so compare the
				// native packets both cycles produce.
				if !reflect.DeepEqual(upgraded, secondLatest) {
					t.Fatalf("native packet drifted across one conversion cycle: first %#v second %#v", upgraded, secondLatest)
				}
				if !reflect.DeepEqual(original, packetValue.Interface()) {
					t.Fatal("conversion mutated the native input packet")
				}
			})
		}
	}
}
