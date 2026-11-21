package packetconv

import "github.com/sandertv/gophertunnel/minecraft/protocol/packet"

// PoolIDs returns the set of packet IDs a pool can construct. Adapters check
// that an outgoing packet has a target representation on every conversion.
// Rebuilding the pool for that membership test allocated one constructor
// closure per entry per packet, which dominated relay cost on versions whose
// packet table is large.
func PoolIDs(pool packet.Pool) map[uint32]struct{} {
	ids := make(map[uint32]struct{}, len(pool))
	for id := range pool {
		ids[id] = struct{}{}
	}
	return ids
}
