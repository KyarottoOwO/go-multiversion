//go:build legacytransport

package multiversion

import (
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/shawtymarco/go-multiversion/mapping"
	"github.com/shawtymarco/go-multiversion/protocols/v1_16_100"
	"github.com/shawtymarco/go-multiversion/protocols/v1_18_0"
	"github.com/shawtymarco/go-multiversion/protocols/v1_18_10"
)

// This opt-in build requires the documented gophertunnel legacy transport fork.
func appendLegacyProtocols(modern []minecraft.Protocol, blocks mapping.BlockRegistry, items []protocol.ItemEntry) ([]minecraft.Protocol, error) {
	legacy, err := buildProtocols([]protocolBuilder{
		{v1_18_10.ID, func() (minecraft.Protocol, error) { return v1_18_10.NewWithRegistries(blocks, items) }},
		{v1_18_0.ID, func() (minecraft.Protocol, error) { return v1_18_0.NewWithRegistries(blocks, items) }},
		{v1_16_100.ID, func() (minecraft.Protocol, error) { return v1_16_100.NewWithRegistries(blocks, items) }},
	}, 0)
	if err != nil {
		return nil, err
	}
	return append(modern, legacy...), nil
}
