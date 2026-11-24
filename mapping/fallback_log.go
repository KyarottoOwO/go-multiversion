package mapping

import (
	"log/slog"
	"sync"
)

// Fallback is one native identifier that a target protocol cannot represent
// exactly. Kind is "block" or "item". Used names the value the adapter
// substituted: a historical block state, or "dropped" when the caller has to
// reject the item.
type Fallback struct {
	Protocol int32
	Kind     string
	Name     string
	Used     string
}

// FallbackLogger receives fallback reports. It is only called for the first
// occurrence of each (protocol, kind, name) in the process, so a hot packet
// path cannot flood a log.
type FallbackLogger func(Fallback)

var (
	fallbackMu   sync.Mutex
	fallbackSeen map[fallbackKey]struct{}
	fallbackSink FallbackLogger
)

type fallbackKey struct {
	protocol int32
	kind     string
	name     string
}

// SetFallbackLogger routes fallback reports and returns the previous logger.
// Passing nil restores the default, which writes one slog record at info level
// per key. A host that already logs conversions can install its own sink and
// keep the same once-per-key rate limit; an adapter never requires one.
func SetFallbackLogger(logger FallbackLogger) FallbackLogger {
	fallbackMu.Lock()
	defer fallbackMu.Unlock()
	previous := fallbackSink
	fallbackSink = logger
	return previous
}

// ResetFallbackReports clears the once-per-key memory. Tests use it to observe
// a report that an earlier test already consumed.
func ResetFallbackReports() {
	fallbackMu.Lock()
	defer fallbackMu.Unlock()
	fallbackSeen = nil
}

// ReportFallback records f unless the same (protocol, kind, name) was already
// reported in this process. It is called from block and item mapping, which run
// per packet, so the duplicate check happens before anything is formatted.
func ReportFallback(f Fallback) {
	fallbackMu.Lock()
	if fallbackSeen == nil {
		fallbackSeen = make(map[fallbackKey]struct{})
	}
	key := fallbackKey{protocol: f.Protocol, kind: f.Kind, name: f.Name}
	if _, seen := fallbackSeen[key]; seen {
		fallbackMu.Unlock()
		return
	}
	fallbackSeen[key] = struct{}{}
	sink := fallbackSink
	fallbackMu.Unlock()
	if sink == nil {
		slog.Info("target protocol cannot represent identifier exactly",
			"protocol", f.Protocol, "kind", f.Kind, "name", f.Name, "used", f.Used)
		return
	}
	sink(f)
}
