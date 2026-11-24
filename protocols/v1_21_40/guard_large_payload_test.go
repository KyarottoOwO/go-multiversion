package v1_21_40

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
)

// TestLargeCommandEnumDecodes decodes an enum near the largest size a real
// server sends (the block and item value lists are the biggest, a few thousand
// entries) through the guarded command path.
func TestLargeCommandEnumDecodes(t *testing.T) {
	const values = 2000
	enum := &protocol.CommandEnum{Type: "test", ValueIndices: make([]uint32, values)}
	for index := range enum.ValueIndices {
		enum.ValueIndices[index] = uint32(index)
	}
	var buffer bytes.Buffer
	marshalCommandEnum827(newWireIO(protocol.NewWriter(&buffer, 0), false, nil), enum, values)
	decoded := &protocol.CommandEnum{}
	marshalCommandEnum827(newWireIO(protocol.NewReader(bytes.NewBuffer(buffer.Bytes()), 0, true), true, nil), decoded, values)
	if len(decoded.ValueIndices) != values {
		t.Fatalf("enum values: got %d, want %d", len(decoded.ValueIndices), values)
	}
}

// TestLargePersonaPieceListDecodes covers the guarded persona list at a size
// larger than any shipped persona pack.
func TestLargePersonaPieceListDecodes(t *testing.T) {
	const pieces = 256
	list := make([]protocol.PersonaPiece, pieces)
	for index := range list {
		list[index] = protocol.PersonaPiece{
			PieceID:   "piece",
			PieceType: 1,
			PackID:    uuid.New(),
			ProductID: "product",
		}
	}
	var buffer bytes.Buffer
	marshalPersonaPieces(newWireIO(protocol.NewWriter(&buffer, 0), false, nil), &list)
	decoded := make([]protocol.PersonaPiece, 0)
	marshalPersonaPieces(newWireIO(protocol.NewReader(bytes.NewBuffer(buffer.Bytes()), 0, true), true, nil), &decoded)
	if len(decoded) != pieces {
		t.Fatalf("persona pieces: got %d, want %d", len(decoded), pieces)
	}
}
