package packetconv

import (
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"strings"
)

// LegacyStartGame excludes native vanilla data-driven definitions. Historical
// clients use their frozen built-in palettes and derive stairs/connections locally.
func LegacyStartGame(pk packet.Packet) packet.Packet {
	game, ok := pk.(*packet.StartGame)
	if !ok {
		return pk
	}
	cloned := *game
	cloned.Blocks = make([]protocol.BlockEntry, 0, len(game.Blocks))
	for _, entry := range game.Blocks {
		if !strings.HasPrefix(entry.Name, "minecraft:") {
			cloned.Blocks = append(cloned.Blocks, entry)
		}
	}
	return &cloned
}

// StopOnlySoundUpdate reports whether the effective native update is Stop.
// Gophertunnel b8bd735 records the client contract: the variant is repeated seven
// times, but only the final (Resume-named) value is applied by the client.
func StopOnlySoundUpdate(pk *packet.ClientboundUpdateSoundData) bool {
	return pk.Resume.Type == protocol.SoundDataUpdateStop
}

// UnsupportedNativePacket excludes additions and boss membership messages that
// cannot carry their historical player identity in the native packet model.
func UnsupportedNativePacket(pk packet.Packet) bool {
	if pk.ID() > packet.IDPartyDestinationCookieResponse {
		return true
	}
	if boss, ok := pk.(*packet.BossEvent); ok {
		switch boss.EventType {
		case packet.BossEventRegisterPlayer, packet.BossEventUnregisterPlayer, packet.BossEventRequest:
			return true
		}
	}
	return false
}
