// Command extract preserves an exact .30 acquisition without enabling it in a server.
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/df-mc/goleveldb/leveldb"
	"github.com/df-mc/goleveldb/leveldb/opt"
	"github.com/sandertv/gophertunnel/minecraft/nbt"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

const archiveHash = "bace5dae627a66020c6a6af3a25881ebda7f585b9fa3dadee23edfb566596574"
const binaryHash = "2c76ff3184906259f3964e5bf2ab9db414f1570b8179a3561637a134a7de72e7"

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func read(path string) []byte { b, e := os.ReadFile(path); must(e); return b }
func digest(b []byte) string  { v := sha256.Sum256(b); return hex.EncodeToString(v[:]) }
func jsonWrite(path string, value any) {
	b, e := json.MarshalIndent(value, "", "  ")
	must(e)
	must(os.WriteFile(path, append(b, '\n'), 0644))
}

// Ordered anonymous compounds let the existing NBT codec retain numeric types
// while sorting keys, so extracted asset bytes do not depend on Go map iteration.
func ordered(value any) any {
	if m, ok := value.(map[string]any); ok {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fields := make([]reflect.StructField, len(keys))
		values := make([]reflect.Value, len(keys))
		for i, k := range keys {
			v := reflect.ValueOf(ordered(m[k]))
			if !v.IsValid() {
				panic("nil NBT value")
			}
			values[i] = v
			fields[i] = reflect.StructField{Name: fmt.Sprintf("F%d", i), Type: v.Type(), Tag: reflect.StructTag(fmt.Sprintf("nbt:%q", k))}
		}
		v := reflect.New(reflect.StructOf(fields)).Elem()
		for i, f := range values {
			v.Field(i).Set(f)
		}
		return v.Interface()
	}
	if list, ok := value.([]any); ok {
		v := make([]any, len(list))
		for i, x := range list {
			v[i] = ordered(x)
		}
		return v
	}
	return value
}
func stateKey(s map[string]any) string {
	b, e := json.Marshal([]any{s["name"], s["states"]})
	must(e)
	return string(b)
}
func decode(b []byte, f func() packet.Packet, shield int32) packet.Packet {
	p := f()
	buf := bytes.NewBuffer(b)
	p.Marshal(protocol.NewReader(buf, shield, false))
	if buf.Len() != 0 {
		panic(fmt.Sprintf("packet %d: %d unread bytes", p.ID(), buf.Len()))
	}
	return p
}

func main() {
	dbPath := flag.String("db", "", "stopped extraction world's database")
	capture := flag.String("capture", "", "vanilla packet capture directory")
	logPath := flag.String("log", "", "script extraction log")
	binaryPath := flag.String("binary", "", "unmodified official Linux BDS binary")
	archive := flag.String("archive", "", "official .30 Linux BDS archive")
	baseline := flag.String("baseline", "", "optional .29 palette for typed comparison")
	out := flag.String("out", "", "new acquisition directory")
	flag.Parse()
	if protocol.CurrentProtocol != 2225 {
		panic("expected native protocol 2225")
	}
	if digest(read(*binaryPath)) != binaryHash || digest(read(*archive)) != archiveHash {
		panic("BDS provenance mismatch")
	}
	if _, e := os.Stat(*out); !os.IsNotExist(e) {
		panic("output must be a new directory")
	}
	must(os.MkdirAll(filepath.Join(*out, "raw"), 0755))
	files := []map[string]any{}
	write := func(path string, b []byte) {
		must(os.WriteFile(filepath.Join(*out, path), b, 0644))
		files = append(files, map[string]any{"path": path, "bytes": len(b), "sha256": digest(b)})
	}
	recordJSON := func(path string, value any) {
		b, e := json.MarshalIndent(value, "", "  ")
		must(e)
		write(path, append(b, '\n'))
	}

	db, e := leveldb.OpenFile(*dbPath, &opt.Options{ReadOnly: true})
	must(e)
	defer db.Close()
	states := map[string]map[string]any{}
	sources := []map[string]any{}
	for _, key := range []string{"structuretemplate_native2225:palette", "structuretemplate_native2225:validated_probe"} {
		raw, e := db.Get([]byte(key), nil)
		must(e)
		buf := bytes.NewBuffer(raw)
		root := map[string]any{}
		must(nbt.NewDecoderWithEncoding(buf, nbt.LittleEndian).Decode(&root))
		if buf.Len() != 0 {
			panic("trailing structure bytes")
		}
		palette := root["structure"].(map[string]any)["palette"].(map[string]any)["default"].(map[string]any)["block_palette"].([]any)
		sources = append(sources, map[string]any{"database_key": key, "bytes": len(raw), "sha256": digest(raw), "entries": len(palette)})
		for _, value := range palette {
			s := value.(map[string]any)
			k := stateKey(s)
			if old, ok := states[k]; ok && !reflect.DeepEqual(old, s) {
				panic("conflicting typed block state")
			}
			states[k] = s
		}
	}
	keys := make([]string, 0, len(states))
	for k := range states {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	names := map[string]bool{}
	hashes := map[uint32]string{}
	propertyTypes := map[string]int{}
	var blockData bytes.Buffer
	enc := nbt.NewEncoder(&blockData)
	for _, key := range keys {
		s := states[key]
		name := s["name"].(string)
		names[name] = true
		props := s["states"].(map[string]any)
		for _, v := range props {
			propertyTypes[fmt.Sprintf("%T", v)]++
		}
		must(enc.Encode(ordered(s)))
		payload, e := nbt.MarshalEncoding(ordered(map[string]any{"name": name, "states": props}), nbt.LittleEndian)
		must(e)
		h := fnv.New32a()
		_, _ = h.Write(payload)
		hash := h.Sum32()
		if name == "minecraft:unknown" {
			hash = 0xfffffffe
		}
		if old, ok := hashes[hash]; ok && old != key {
			panic("block hash collision")
		}
		hashes[hash] = key
	}
	if len(states) != 23960 || len(names) != 1480 {
		panic(fmt.Sprintf("incomplete palette: %d states / %d blocks", len(states), len(names)))
	}
	write("block_states.nbt", blockData.Bytes())
	equalBaseline := false
	if *baseline != "" {
		b := bytes.NewBuffer(read(*baseline))
		d := nbt.NewDecoder(b)
		count := 0
		for b.Len() > 0 {
			s := map[string]any{}
			must(d.Decode(&s))
			if !reflect.DeepEqual(s, states[stateKey(s)]) {
				panic(".29/.30 typed state differs")
			}
			count++
		}
		equalBaseline = count == len(states)
	}

	definitions := []struct {
		name    string
		factory func() packet.Packet
	}{
		{"item_registry", func() packet.Packet { return &packet.ItemRegistry{} }},
		{"start_game", func() packet.Packet { return &packet.StartGame{} }},
		{"creative_content", func() packet.Packet { return &packet.CreativeContent{} }},
		{"crafting_data", func() packet.Packet { return &packet.CraftingData{} }},
		{"biome_definitions", func() packet.Packet { return &packet.BiomeDefinitionList{} }},
		{"entity_identifiers", func() packet.Packet { return &packet.AvailableActorIdentifiers{} }},
	}
	shield := int32(0)
	items := map[int32]string{}
	itemNBT := map[string]any{}
	packetCounts := map[string]any{}
	roundtrips := []string{}
	creativeHashes := 0
	for _, def := range definitions {
		raw := read(filepath.Join(*capture, def.name+"-01.bin"))
		p := decode(raw, def.factory, shield)
		write("raw/"+def.name+".bin", raw)
		switch p := p.(type) {
		case *packet.ItemRegistry:
			for _, item := range p.Items {
				rid := int32(item.RuntimeID)
				if _, ok := items[rid]; ok {
					panic("duplicate item runtime ID")
				}
				if _, ok := itemNBT[item.Name]; ok {
					panic("duplicate item name")
				}
				items[rid] = item.Name
				itemNBT[item.Name] = map[string]any{"runtime_id": rid, "component_based": item.ComponentBased, "version": item.Version, "data": item.Data}
				if item.Name == "minecraft:shield" {
					shield = rid
				}
			}
			b, e := nbt.Marshal(ordered(itemNBT))
			must(e)
			write("vanilla_items.nbt", b)
			packetCounts[def.name] = len(items)
		case *packet.StartGame:
			packetCounts[def.name] = map[string]any{"data_driven_blocks": len(p.Blocks), "experiments": len(p.Experiments), "hashes": p.UseBlockNetworkIDHashes}
			data := map[string]any{}
			for _, block := range p.Blocks {
				data[block.Name] = block.Properties
			}
			b, e := nbt.Marshal(ordered(data))
			must(e)
			write("data_driven_blocks.nbt", b)
		case *packet.CreativeContent:
			for _, entry := range p.Items {
				if int(entry.GroupIndex) >= len(p.Groups) {
					panic("creative group outside catalogue")
				}
				if _, ok := items[entry.Item.NetworkID]; !ok {
					panic("creative item outside registry")
				}
				if entry.Item.BlockRuntimeID != 0 {
					if _, ok := hashes[uint32(entry.Item.BlockRuntimeID)]; !ok {
						panic(fmt.Sprintf("creative block hash absent: %d", entry.Item.BlockRuntimeID))
					}
					creativeHashes++
				}
			}
			packetCounts[def.name] = map[string]int{"groups": len(p.Groups), "items": len(p.Items), "validated_block_hashes": creativeHashes}
		default:
			value := reflect.ValueOf(p).Elem()
			counts := map[string]int{}
			for i := 0; i < value.NumField(); i++ {
				if value.Field(i).Kind() == reflect.Slice {
					counts[value.Type().Field(i).Name] = value.Field(i).Len()
				}
			}
			packetCounts[def.name] = counts
		}
		var encoded bytes.Buffer
		p.Marshal(protocol.NewWriter(&encoded, shield))
		back := decode(encoded.Bytes(), def.factory, shield)
		if !reflect.DeepEqual(p, back) {
			panic("typed packet roundtrip mismatch: " + def.name)
		}
		roundtrips = append(roundtrips, def.name)
	}
	if len(items) != 2080 || shield == 0 {
		panic("incomplete item registry")
	}

	log := read(*logPath)
	scanner := bufio.NewScanner(bytes.NewReader(log))
	scanner.Buffer(make([]byte, 4096), 8<<20)
	domains := []any{}
	blockItems := map[string]any{}
	tags := map[string]any{}
	candidates := []any{}
	var summary map[string]any
	for scanner.Scan() {
		line := scanner.Text()
		for _, marker := range []string{"NATIVE2225_DOMAIN ", "NATIVE2225_BLOCK_ITEM ", "NATIVE2225_ITEM_TAGS ", "NATIVE2225_CANDIDATE ", "NATIVE2225_DONE "} {
			_, body, ok := strings.Cut(line, marker)
			if !ok {
				continue
			}
			var value map[string]any
			must(json.Unmarshal([]byte(body), &value))
			switch marker {
			case "NATIVE2225_DOMAIN ":
				domains = append(domains, value)
			case "NATIVE2225_BLOCK_ITEM ":
				blockItems[value["name"].(string)] = value
			case "NATIVE2225_ITEM_TAGS ":
				tags[value["name"].(string)] = value["tags"]
			case "NATIVE2225_CANDIDATE ":
				if value["name"] == nil {
					panic("candidate did not resolve in .30")
				}
				candidates = append(candidates, value)
			case "NATIVE2225_DONE ":
				summary = value
			}
		}
	}
	must(scanner.Err())
	if summary == nil || len(blockItems) != 1480 || len(tags) != 2080 || len(candidates) != 20 {
		panic(fmt.Sprintf("incomplete script evidence: blocks=%d items=%d candidates=%d", len(blockItems), len(tags), len(candidates)))
	}
	recordJSON("block_state_domains.json", domains)
	recordJSON("block_to_item.json", blockItems)
	recordJSON("item_tags.json", tags)
	recordJSON("script_validation.json", map[string]any{"summary": summary, "native_resolved_candidates": candidates, "structure_sources": sources})
	recordJSON("validation.json", map[string]any{"block_states": len(states), "block_identifiers": len(names), "property_types": propertyTypes, "typed_block_states_equal_29": equalBaseline, "packets": packetCounts, "packet_type_value_roundtrips": roundtrips, "native_protocol": 2225, "unread_packet_bytes": 0})
	sort.Slice(files, func(i, j int) bool { return files[i]["path"].(string) < files[j]["path"].(string) })
	jsonWrite(filepath.Join(*out, "files.json"), files)
	fmt.Printf("Extracted %d block states, %d items, %d verified creative block hashes\n", len(states), len(items), creativeHashes)
}
