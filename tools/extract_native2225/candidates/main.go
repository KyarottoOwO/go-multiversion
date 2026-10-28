package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/df-mc/goleveldb/leveldb"
	"github.com/df-mc/goleveldb/leveldb/opt"
	"github.com/sandertv/gophertunnel/minecraft/nbt"
	"os"
	"path/filepath"
	"sort"
)

func read(path string) map[string]map[string]any {
	if info, e := os.Stat(path); e == nil && info.IsDir() {
		db, e := leveldb.OpenFile(path, &opt.Options{ReadOnly: true})
		if e != nil {
			panic(e)
		}
		defer db.Close()
		raw, e := db.Get([]byte("structuretemplate_native2225:palette"), nil)
		if e != nil {
			panic(e)
		}
		var root map[string]any
		if e := nbt.UnmarshalEncoding(raw, &root, nbt.LittleEndian); e != nil {
			panic(e)
		}
		palette := root["structure"].(map[string]any)["palette"].(map[string]any)["default"].(map[string]any)["block_palette"].([]any)
		r := map[string]map[string]any{}
		for _, value := range palette {
			s := value.(map[string]any)
			k, _ := json.Marshal([]any{s["name"], s["states"]})
			r[string(k)] = s
		}
		return r
	}
	b, e := os.ReadFile(path)
	if e != nil {
		panic(e)
	}
	buf := bytes.NewBuffer(b)
	d := nbt.NewDecoder(buf)
	r := map[string]map[string]any{}
	for buf.Len() > 0 {
		var s map[string]any
		if e := d.Decode(&s); e != nil {
			panic(e)
		}
		k, _ := json.Marshal([]any{s["name"], s["states"]})
		r[string(k)] = s
	}
	return r
}
func main() {
	if len(os.Args) != 4 {
		panic("usage: candidates BASELINE_NBT PRIMARY_NBT_OR_STOPPED_DB OUTPUT_MCSTRUCTURE")
	}
	a, b := read(os.Args[1]), read(os.Args[2])
	keys := []string{}
	for k := range a {
		if _, ok := b[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	palette := []any{}
	indices := []int32{}
	void := []int32{}
	for i, k := range keys {
		palette = append(palette, a[k])
		indices = append(indices, int32(i))
		void = append(void, -1)
	}
	root := map[string]any{"format_version": int32(1), "size": []int32{int32(len(keys)), 1, 1}, "structure_world_origin": []int32{0, 0, 0}, "structure": map[string]any{"entities": []any{}, "block_indices": []any{indices, void}, "palette": map[string]any{"default": map[string]any{"block_palette": palette, "block_position_data": map[string]any{}}}}}
	raw, e := nbt.MarshalEncoding(root, nbt.LittleEndian)
	if e != nil {
		panic(e)
	}
	_ = os.MkdirAll(filepath.Dir(os.Args[3]), 0755)
	if e := os.WriteFile(os.Args[3], raw, 0644); e != nil {
		panic(e)
	}
	fmt.Println("candidate states", len(keys))
}
