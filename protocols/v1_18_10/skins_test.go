package v1_18_10

import (
	"bytes"
	"image/color"
	"testing"

	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func TestParseLegacyColourShorthandZero(t *testing.T) {
	if got := parseLegacyColour(nil, "#0"); got != (color.RGBA{}) {
		t.Fatalf("parseLegacyColour(#0) = %#v", got)
	}
}

// TestEmptySkinColourIsReadable checks that the adapter can decode the empty
// skin colour it writes itself. marshalSkin emits an empty string for a skin
// without data (NameTagOnly players and servers that never set a skin), and the
// reader used to reject that value instead of decoding it as a zero colour.
func TestEmptySkinColourIsReadable(t *testing.T) {
	p := New().(*Protocol)
	original := &packet.PlayerSkin{UUID: uuid.MustParse("123e4567-e89b-12d3-a456-426614174000")}
	marshal, ok := packetMarshals[packet.IDPlayerSkin]
	if !ok {
		t.Fatal("protocol has no PlayerSkin marshal")
	}
	var buffer bytes.Buffer
	marshal(newWireIO(p.NewWriter(&buffer, -1), false, nil), original)
	decoded := &packet.PlayerSkin{}
	marshal(newWireIO(p.NewReader(bytes.NewReader(buffer.Bytes()), -1, false), true, nil), decoded)
	if decoded.UUID != original.UUID {
		t.Fatalf("skin owner: got %v, want %v", decoded.UUID, original.UUID)
	}
	if decoded.Skin.SkinColour != (color.RGBA{}) {
		t.Fatalf("empty skin colour decoded as %#v", decoded.Skin.SkinColour)
	}
}
