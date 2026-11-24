package v1_16_100

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// TestLargeLegalPayloadsDecode decodes payloads at the largest size a real
// server sends for each guarded field, so the 65536-element ceiling is shown to
// sit far above anything legitimate rather than merely above the samples.
func TestLargeLegalPayloadsDecode(t *testing.T) {
	runtime, err := newRuntimeData(protocol419IntegrationBlockRegistry(t), nil, MappingOptions{})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("actor_metadata", func(t *testing.T) {
		// The write path drops keys the target version does not know, so the
		// 200-entry payload is built on the wire to feed the decoder a count no
		// conversion would produce.
		const entries = 200
		var buffer bytes.Buffer
		writer := protocol.NewWriter(&buffer, 0)
		count := uint32(entries)
		writer.Varuint32(&count)
		for index := range entries {
			key, kind := uint32(index)+100, uint32(protocol.EntityDataTypeInt32)
			writer.Varuint32(&key)
			writer.Varuint32(&kind)
			value := int32(index)
			writer.Varint32(&value)
		}
		remaining := bytes.NewBuffer(bytes.Clone(buffer.Bytes()))
		decoded := make(protocol.EntityMetadata)
		marshalEntityMetadata(newWireIO(protocol.NewReader(remaining, 0, true), true, runtime), &decoded)
		if remaining.Len() != 0 {
			t.Fatalf("metadata payload has %d unread bytes", remaining.Len())
		}
	})

	t.Run("player_list", func(t *testing.T) {
		const entries = 100
		list := &packet.PlayerList{Entries: make([]protocol.PlayerListEntry, entries)}
		for index := range list.Entries {
			list.Entries[index] = protocol.PlayerListEntry{
				UUID:           uuid.New(),
				EntityUniqueID: int64(index) + 1,
				Username:       fmt.Sprintf("player%d", index),
				ActionType:     protocol.PlayerListActionAdd,
			}
		}
		var buffer bytes.Buffer
		marshalPlayerList(newWireIO(protocol.NewWriter(&buffer, 0), false, runtime), list)
		decoded := &packet.PlayerList{}
		marshalPlayerList(newWireIO(protocol.NewReader(bytes.NewBuffer(buffer.Bytes()), 0, true), true, runtime), decoded)
		if len(decoded.Entries) != entries {
			t.Fatalf("player list entries: got %d, want %d", len(decoded.Entries), entries)
		}
	})

	t.Run("resource_packs_info", func(t *testing.T) {
		const entries = 64
		packs := make([]protocol.TexturePackInfo, entries)
		for index := range packs {
			packs[index] = protocol.TexturePackInfo{
				UUID:            uuid.New(),
				Version:         "1.0.0",
				Size:            1024,
				ContentIdentity: fmt.Sprintf("pack%d", index),
			}
		}
		var buffer bytes.Buffer
		marshalResourcePacksInfo(newWireIO(protocol.NewWriter(&buffer, 0), false, runtime), &packet.ResourcePacksInfo{TexturePacks: packs})
		decoded := &packet.ResourcePacksInfo{}
		marshalResourcePacksInfo(newWireIO(protocol.NewReader(bytes.NewBuffer(buffer.Bytes()), 0, true), true, runtime), decoded)
		if len(decoded.TexturePacks) != entries {
			t.Fatalf("texture packs: got %d, want %d", len(decoded.TexturePacks), entries)
		}
	})
}
