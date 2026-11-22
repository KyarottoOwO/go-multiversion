package compat

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sandertv/gophertunnel/minecraft"
)

// TestScanNestedCorpus is an opt-in investigation helper, not part of the
// suite: it replays every entry of a cached fuzz corpus through the same decode
// and conversion path as FuzzNestedFields, one entry at a time, while a watchdog
// watches the heap. A worker that dies without a Go panic leaves no crasher file
// behind, so this is how the offending input is identified: the current entry is
// written to MV_SCAN_CURRENT before it runs, and the watchdog exits as soon as
// the heap passes MV_SCAN_LIMIT_MB.
//
// usage:
//
//	MV_SCAN_CORPUS=<dir> MV_SCAN_CURRENT=<file> go test ./internal/compat -run TestScanNestedCorpus -v
func TestScanNestedCorpus(t *testing.T) {
	dir := os.Getenv("MV_SCAN_CORPUS")
	if dir == "" {
		t.Skip("set MV_SCAN_CORPUS to a fuzz corpus directory")
	}
	currentPath := os.Getenv("MV_SCAN_CURRENT")
	limitMB := 1500
	if raw := os.Getenv("MV_SCAN_LIMIT_MB"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			t.Fatalf("bad MV_SCAN_LIMIT_MB: %v", err)
		}
		limitMB = parsed
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}
	adapters := catalogue(t)

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		var stats runtime.MemStats
		for {
			select {
			case <-stop:
				return
			case <-time.After(50 * time.Millisecond):
			}
			runtime.ReadMemStats(&stats)
			if int(stats.HeapAlloc>>20) > limitMB {
				fmt.Printf("WATCHDOG heap=%dMB limit=%dMB entry=%s\n", stats.HeapAlloc>>20, limitMB, currentPath)
				os.Exit(3)
			}
		}
	}()

	for i, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		protocolID, packetID, listener, data, err := parseCorpusEntry(raw)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		if currentPath != "" {
			_ = os.WriteFile(currentPath, raw, 0o600)
		}
		if i%250 == 0 {
			fmt.Printf("entry %d/%d %s\n", i, len(entries), entry.Name())
		}
		nestedFieldsReplay(adapters, protocolID, packetID, listener, data)
		runtime.GC()
	}
	fmt.Printf("SCAN DONE: %d entries under %dMB\n", len(entries), limitMB)
}

// nestedFieldsReplay mirrors the body of FuzzNestedFields. Keep the two in step:
// the point of this helper is to run exactly what the fuzzer runs.
func nestedFieldsReplay(adapters []minecraft.Protocol, protocolID, packetID uint32, listener bool, data []byte) {
	defer func() { _ = recover() }()
	p := findProtocol(adapters, protocolID)
	if p == nil {
		return
	}
	constructor, ok := p.Packets(listener)[packetID]
	if !ok {
		return
	}
	decoded := constructor()
	decoded.Marshal(p.NewReader(bytes.NewReader(data), 0, true))
	for _, latest := range p.ConvertToLatest(decoded, nil) {
		if latest == nil {
			return
		}
		for _, back := range p.ConvertFromLatest(latest, nil) {
			if back != nil {
				var sink bytes.Buffer
				back.Marshal(p.NewWriter(&sink, 0))
			}
		}
	}
}

// parseCorpusEntry reads one "go test fuzz v1" corpus file: one Go literal per
// line, in the argument order of the fuzz target.
func parseCorpusEntry(raw []byte) (protocolID, packetID uint32, listener bool, data []byte, err error) {
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 5 || lines[0] != "go test fuzz v1" {
		return 0, 0, false, nil, fmt.Errorf("unexpected corpus header %q", lines[0])
	}
	u32 := func(line string) (uint32, error) {
		if !strings.HasPrefix(line, "uint32(") || !strings.HasSuffix(line, ")") {
			return 0, fmt.Errorf("want uint32, got %q", line)
		}
		value, err := strconv.ParseUint(strings.TrimSuffix(strings.TrimPrefix(line, "uint32("), ")"), 10, 32)
		return uint32(value), err
	}
	if protocolID, err = u32(lines[1]); err != nil {
		return 0, 0, false, nil, err
	}
	if packetID, err = u32(lines[2]); err != nil {
		return 0, 0, false, nil, err
	}
	switch lines[3] {
	case "bool(false)":
		listener = false
	case "bool(true)":
		listener = true
	default:
		return 0, 0, false, nil, fmt.Errorf("want bool, got %q", lines[3])
	}
	if !strings.HasPrefix(lines[4], "[]byte(") {
		return 0, 0, false, nil, fmt.Errorf("want []byte, got %q", lines[4])
	}
	quoted := strings.TrimSuffix(strings.TrimPrefix(lines[4], "[]byte("), ")")
	unquoted, err := strconv.Unquote(quoted)
	if err != nil {
		return 0, 0, false, nil, err
	}
	return protocolID, packetID, listener, []byte(unquoted), nil
}
