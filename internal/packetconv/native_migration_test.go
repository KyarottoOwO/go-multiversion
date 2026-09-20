package packetconv

import (
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"testing"
)

func TestLegacySoundUsesEffectiveNativeOperation(t *testing.T) {
	pk := &packet.ClientboundUpdateSoundData{SetVolume: protocol.SoundDataUpdate{Type: protocol.SoundDataUpdateSetVolume, Volume: .5}}
	if !StopOnlySoundUpdate(pk) {
		t.Fatal("ignored earlier fields changed the final Stop operation")
	}
	pk.Resume = protocol.SoundDataUpdate{Type: protocol.SoundDataUpdateSetVolume, Volume: .5}
	if StopOnlySoundUpdate(pk) {
		t.Fatal("effective volume change was misrepresented as Stop")
	}
}
