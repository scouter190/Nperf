package hrtime

import (
	"testing"
	"time"
)

func TestMonotonicAndFine(t *testing.T) {
	prev := Now()
	distinct := map[int64]struct{}{prev: {}}
	start := time.Now()
	for time.Since(start) < 5*time.Millisecond {
		n := Now()
		if n < prev {
			t.Fatalf("retrocede: %d < %d", n, prev)
		}
		prev = n
		distinct[n] = struct{}{}
	}
	// Resolución fina: muchos valores distintos en pocos ms (un reloj grueso daría ~1-10).
	if len(distinct) < 20 {
		t.Fatalf("resolución gruesa: %d valores distintos", len(distinct))
	}
}

func TestSince(t *testing.T) {
	t0 := Now()
	time.Sleep(20 * time.Millisecond)
	if d := Since(t0); d < 15*time.Millisecond || d > 2*time.Second {
		t.Fatalf("Since = %v", d)
	}
}
