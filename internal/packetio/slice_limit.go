package packetio

import (
	"fmt"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
)

// MaxSliceElements bounds a wire-supplied element count before it is used as an
// allocation size. It is far above any legal list in these packets (the largest
// are command index tables and player lists) and far below a count that could
// exhaust memory: 2^16 elements of a 100-byte struct is about 6.5 MB.
const MaxSliceElements = 1 << 16

// sliceLimiter is implemented by gophertunnel readers and by the adapters'
// directional readers. SliceLength panics with a protocol error when the count
// does not fit in the remaining payload.
type sliceLimiter interface {
	SliceLength(value uint32, max uint32)
}

// CheckSliceLength rejects n when it exceeds the remaining payload or
// MaxSliceElements. Callers must call it before make: a connection recovers a
// panicked protocol error, but allocating an attacker-controlled count first
// kills the process with a fatal out-of-memory error instead.
func CheckSliceLength(io protocol.IO, n uint32) {
	if limiter, ok := io.(sliceLimiter); ok {
		limiter.SliceLength(n, MaxSliceElements)
		return
	}
	if n > MaxSliceElements {
		panic(fmt.Errorf("slice length %d exceeds the %d element limit", n, MaxSliceElements))
	}
}
