package port

import (
	"net"
	"strconv"
	"testing"
)

func TestListenDeterministicPerRoot(t *testing.T) {
	ln1, p1, err := Listen("/project/a", DefaultBase, DefaultSpan)
	if err != nil {
		t.Fatalf("listen a: %v", err)
	}
	ln1.Close()

	ln2, p2, err := Listen("/project/a", DefaultBase, DefaultSpan)
	if err != nil {
		t.Fatalf("listen a again: %v", err)
	}
	ln2.Close()

	if p1 != p2 {
		t.Fatalf("same root should map to same port, got %d vs %d", p1, p2)
	}

	ln3, p3, err := Listen("/project/b", DefaultBase, DefaultSpan)
	if err != nil {
		t.Fatalf("listen b: %v", err)
	}
	ln3.Close()

	if p3 == p1 {
		t.Fatalf("different roots should (usually) map to different ports, both %d", p3)
	}
}

func TestListenFallsBackWhenPortTaken(t *testing.T) {
	// occupy the deterministic port for /blocked
	first, expected, err := Listen("/blocked", DefaultBase, DefaultSpan)
	if err != nil {
		t.Fatalf("first listen: %v", err)
	}
	defer first.Close()

	// second listen must pick a different, free port
	second, actual, err := Listen("/blocked", DefaultBase, DefaultSpan)
	if err != nil {
		t.Fatalf("second listen: %v", err)
	}
	defer second.Close()

	if actual == expected {
		t.Fatalf("expected a different port when the deterministic one is taken, got %d", actual)
	}
}

func TestListenReturnsBoundListener(t *testing.T) {
	ln, p, err := Listen("/probe", DefaultBase, DefaultSpan)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	// the listener must be actually bound
	conn, err := net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(p))
	if err != nil {
		t.Fatalf("port %d not bound: %v", p, err)
	}
	conn.Close()
}
