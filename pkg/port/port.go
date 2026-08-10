package port

import (
	"fmt"
	"hash/fnv"
	"net"
	"strconv"
)

// Default base/span for MCP streamable-http listeners. The range sits well
// above common dev ports (3000-9999) to reduce collisions.
const (
	DefaultBase = 45100
	DefaultSpan = 2000
)

// Listen binds a listener on a deterministic, per-project port derived from
// root. The deterministic candidate is tried first; if it is already taken,
// Listen scans forward within [base, base+span) for the first free port.
// The returned listener already holds the port, so there is no TOCTOU window
// between selection and serving.
func Listen(root string, base, span int) (net.Listener, int, error) {
	if span <= 0 {
		return nil, 0, fmt.Errorf("invalid port span %d", span)
	}
	offset := hashOffset(root, span)

	var lastErr error
	for i := 0; i < span; i++ {
		port := base + (offset+i)%span
		ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
		if err == nil {
			return ln, port, nil
		}
		lastErr = err
	}
	return nil, 0, fmt.Errorf("no free port in [%d, %d): %w", base, base+span, lastErr)
}

// hashOffset maps root to a stable offset in [0, span).
func hashOffset(root string, span int) int {
	h := fnv.New32a()
	h.Write([]byte(root))
	return int(h.Sum32() % uint32(span))
}
