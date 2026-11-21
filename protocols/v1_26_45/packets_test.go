package v1_26_45

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// TestPlayerAuthInputItemStackRequestRoundTrip covers the nested stack request
// inside PlayerAuthInput. Its native Marshal allocates slices through
// gophertunnel's sliceReader interface, which the bare wireIO does not expose,
// so a request with any action or filter string used to panic while decoding.
func TestPlayerAuthInputItemStackRequestRoundTrip(t *testing.T) {
	p := New().(*Protocol)
	request := protocol.ItemStackRequest{
		RequestID:     7,
		Actions:       []protocol.StackRequestAction{&protocol.TakeStackRequestAction{}},
		FilterStrings: []string{"filter"},
	}
	input := &packet.PlayerAuthInput{ItemStackRequest: protocol.Option(request), InputMode: packet.InputModeMouse, PlayMode: packet.PlayModeNormal}
	marshal, ok := packetMarshals[packet.IDPlayerAuthInput]
	if !ok {
		t.Fatal("protocol 2169 has no PlayerAuthInput marshal")
	}
	var wire bytes.Buffer
	marshal(newWireIO(p.NewWriter(&wire, -1), false), input)
	decoded := &packet.PlayerAuthInput{}
	marshal(newWireIO(p.NewReader(bytes.NewReader(wire.Bytes()), -1, true), true), decoded)
	got, present := decoded.ItemStackRequest.Value()
	if !present {
		t.Fatal("stack request was lost")
	}
	if got.RequestID != request.RequestID || len(got.Actions) != 1 || len(got.FilterStrings) != 1 || got.FilterStrings[0] != "filter" {
		t.Fatalf("stack request changed: %#v", got)
	}
	if reflect.TypeOf(got.Actions[0]) != reflect.TypeOf(request.Actions[0]) {
		t.Fatalf("stack request action changed type: got %T, want %T", got.Actions[0], request.Actions[0])
	}
	if value, _ := input.ItemStackRequest.Value(); len(value.Actions) != 1 || len(value.FilterStrings) != 1 {
		t.Fatalf("input packet was mutated: %#v", value)
	}
}
