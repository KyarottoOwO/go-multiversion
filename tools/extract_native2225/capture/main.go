package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol/login"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func main() {
	addr := flag.String("address", "127.0.0.1:19172", "vanilla BDS address")
	network := flag.String("network", "raknet", "network")
	out := flag.String("output", "capture", "output directory")
	flag.Parse()
	if minecraft.DefaultProtocol.ID() != 2225 {
		panic("wrong native protocol")
	}
	if err := os.MkdirAll(*out, 0755); err != nil {
		panic(err)
	}
	allow := map[uint32]string{packet.IDStartGame: "start_game", packet.IDItemRegistry: "item_registry", packet.IDCreativeContent: "creative_content", packet.IDCraftingData: "crafting_data", packet.IDBiomeDefinitionList: "biome_definitions", packet.IDAvailableActorIdentifiers: "entity_identifiers"}
	var mu sync.Mutex
	counts := map[string]int{}
	records := []map[string]any{}
	dialer := minecraft.Dialer{IdentityData: login.IdentityData{DisplayName: "DataExtraction"},
		PacketFunc: func(header packet.Header, payload []byte, src, dst net.Addr) {
			name, ok := allow[header.PacketID]
			if !ok {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			counts[name]++
			filename := fmt.Sprintf("%s-%02d.bin", name, counts[name])
			data := append([]byte(nil), payload...)
			if err := os.WriteFile(filepath.Join(*out, filename), data, 0644); err != nil {
				panic(err)
			}
			hash := sha256.Sum256(data)
			records = append(records, map[string]any{"packet_id": header.PacketID, "path": filename, "bytes": len(data), "sha256": hex.EncodeToString(hash[:])})
			fmt.Printf("captured %s (%d bytes)\n", filename, len(data))
		}}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	conn, err := dialer.DialContext(ctx, *network, *addr)
	if err == nil {
		defer conn.Close()
		err = conn.DoSpawnContext(ctx)
		if err == nil {
			go func() { <-ctx.Done(); _ = conn.Close() }()
			for {
				_, err = conn.ReadPacket()
				if err != nil {
					break
				}
				mu.Lock()
				complete := counts["creative_content"] > 0 && counts["crafting_data"] > 0 && counts["item_registry"] > 0 && counts["biome_definitions"] > 0
				mu.Unlock()
				if complete {
					err = nil
					break
				}
			}
		}
	}
	mu.Lock()
	defer mu.Unlock()
	result := map[string]any{"native_protocol": minecraft.DefaultProtocol.ID(), "native_version": minecraft.DefaultProtocol.Ver(), "files": records}
	if err != nil {
		result["error"] = err.Error()
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	_ = os.WriteFile(filepath.Join(*out, "capture.json"), append(data, '\n'), 0644)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
