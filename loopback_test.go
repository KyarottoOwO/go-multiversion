package multiversion_test

import (
	"context"
	"os"
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
)

// TestLoopbackJoin dials every advertised protocol through a real RakNet
// listener on the loopback interface: negotiation, login, StartGame, spawn and a
// text exchange. Protocol 2168 is dialled twice, once as 1.26.40 and once as
// 1.26.44, because those releases share the protocol ID but select different
// SetScore layouts from the login GameVersion. The exact bytes of both layouts
// are pinned separately by TestSetScoreOraclePayloads; this harness proves the
// selected layout is reachable through a real connection.
//
// It is opt-in because it opens UDP sockets: MV_LOOPBACK=1.
func TestLoopbackJoin(t *testing.T) {
	if os.Getenv("MV_LOOPBACK") == "" {
		t.Skip("set MV_LOOPBACK=1 to run the loopback joins")
	}
	registry, items := perfFixture(t)
	adapters, err := multiversion.ProtocolsWithRegistries(registry, items)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := (minecraft.ListenConfig{
		AuthenticationDisabled: true,
		AcceptedProtocols:      adapters,
	}).Listen("raknet", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	address := listener.Addr().String()

	type target struct {
		adapter minecraft.Protocol
		version string
	}
	targets := make([]target, 0, len(adapters)+1)
	for _, adapter := range adapters {
		// 419, 475 and 486 negotiate RakNet v10 / Login-first flate, which only
		// the legacy transport fork implements; without it the dial just times
		// out and leaves half-open sessions behind for the next case.
		if isLegacyLoopbackAdapter(adapter.ID()) && !legacyTransportEnabled() {
			continue
		}
		targets = append(targets, target{adapter: adapter, version: adapter.Ver()})
	}
	// 1.26.40 selects the single-optional SetScore layout for protocol 2168.
	for _, adapter := range adapters {
		if adapter.ID() == 2168 {
			targets = append(targets, target{adapter: adapter, version: "1.26.40"})
		}
	}

	for _, current := range targets {
		t.Run(strconv.Itoa(int(current.adapter.ID()))+"/"+current.version, func(t *testing.T) {
			runLoopbackJoin(t, listener, address, current.adapter, current.version, items)
		})
	}
}

// runLoopbackJoin performs one complete join. The server side runs in this
// process: it accepts, sends StartGame, waits for the client's ready message,
// answers it and then waits for the client to close.
func runLoopbackJoin(t *testing.T, listener *minecraft.Listener, address string, adapter minecraft.Protocol, version string, items []protocol.ItemEntry) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	serverErr := make(chan error, 1)
	go func() {
		raw, err := listener.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		conn, ok := raw.(*minecraft.Conn)
		if !ok {
			_ = raw.Close()
			serverErr <- errUnexpectedConn
			return
		}
		defer conn.Close()
		if err := conn.StartGameContext(ctx, minecraft.GameData{
			WorldName:                    "loopback",
			BaseGameVersion:              "1.26.60",
			PlayerGameMode:               packet.GameTypeCreative,
			WorldGameMode:                packet.GameTypeCreative,
			PlayerPosition:               mgl32.Vec3{0, 64, 0},
			WorldSpawn:                   protocol.BlockPos{0, 64, 0},
			Items:                        items,
			ServerAuthoritativeInventory: true,
			PlayerMovementSettings:       protocol.PlayerMovementSettings{ServerAuthoritativeBlockBreaking: true},
		}); err != nil {
			serverErr <- err
			return
		}
		_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		for {
			pk, err := conn.ReadPacket()
			if err != nil {
				serverErr <- err
				return
			}
			if text, ok := pk.(*packet.Text); ok && text.Message == "loopback-ready" {
				break
			}
		}
		if err := conn.WritePacket(&packet.Text{TextType: packet.TextTypeSystem, Message: "loopback-pong"}); err != nil {
			serverErr <- err
			return
		}
		// Wait for the client to disconnect so the pong is flushed.
		_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		for {
			if _, err := conn.ReadPacket(); err != nil {
				break
			}
		}
		serverErr <- nil
	}()

	dialer := minecraft.Dialer{
		Protocol:     adapter,
		IdentityData: login.IdentityData{Identity: uuid.NewString(), DisplayName: "Loopback"},
		ClientData:   login.ClientData{GameVersion: version},
	}
	client, err := dialer.DialContext(ctx, "raknet", address)
	if err != nil {
		t.Fatalf("dial as %s: %v", version, err)
	}
	defer client.Close()
	if err := client.DoSpawnContext(ctx); err != nil {
		t.Fatalf("spawn as %s: %v", version, err)
	}
	if err := client.WritePacket(&packet.Text{TextType: packet.TextTypeSystem, Message: "loopback-ready"}); err != nil {
		t.Fatalf("write ready: %v", err)
	}
	_ = client.SetReadDeadline(time.Now().Add(30 * time.Second))
	for {
		pk, err := client.ReadPacket()
		if err != nil {
			t.Fatalf("read pong: %v", err)
		}
		if text, ok := pk.(*packet.Text); ok && text.Message == "loopback-pong" {
			break
		}
	}
	_ = client.Close()
	if err := <-serverErr; err != nil {
		t.Fatalf("server side: %v", err)
	}
}

// isLegacyLoopbackAdapter reports whether the protocol needs the legacy
// transport fork to complete a handshake.
func isLegacyLoopbackAdapter(id int32) bool {
	switch id {
	case 419, 475, 486:
		return true
	}
	return false
}

type loopbackError string

func (e loopbackError) Error() string { return string(e) }

const errUnexpectedConn = loopbackError("listener accepted a non-*minecraft.Conn")
