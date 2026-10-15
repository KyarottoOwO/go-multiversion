package mapping

// Pre12650Biome maps the newly introduced Dappled Forest (195) to Forest (4).
// IDs are locked to Dragonfly 4c7b5074 and its outgoing 26.45 registry. All other
// IDs retain the established adapter policy; this is a native-upgrade delta.
// Protocol 419 does not use this mapping: its client resolves biome bytes
// against its own small registry and crashes on an unknown ID, so it maps
// through Biome419 instead.
func Pre12650Biome(id uint32) (uint32, bool) {
	if id == 195 {
		return 4, true
	}
	return id, true
}
