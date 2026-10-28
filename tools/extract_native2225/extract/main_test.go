package main

import (
	"bytes"
	"github.com/sandertv/gophertunnel/minecraft/nbt"
	"reflect"
	"testing"
)

func TestOrderedNBTIsDeterministicAndRetainsTypes(t *testing.T) {
	original := map[string]any{"name": "minecraft:test", "states": map[string]any{"int": int32(2), "byte": uint8(1), "text": "north"}, "version": int32(18168865), "nested": []any{map[string]any{"x": int16(-2)}, map[string]any{"y": int64(12)}}}
	var first []byte
	for range 25 {
		data, err := nbt.Marshal(ordered(original))
		if err != nil {
			t.Fatal(err)
		}
		if first == nil {
			first = data
		} else if !bytes.Equal(first, data) {
			t.Fatal("unstable NBT key ordering")
		}
		var decoded map[string]any
		if err := nbt.Unmarshal(data, &decoded); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(original, decoded) {
			t.Fatalf("NBT type/value changed: %#v", decoded)
		}
	}
}
