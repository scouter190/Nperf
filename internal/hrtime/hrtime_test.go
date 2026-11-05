package hrtime

import (
	"testing"
	"time"
)

func TestMonotonicAndFine(t *testing.T) {
	const maxReads = 5_000_000
	const wantIncrements = 200
	prev := Now()
	var minInc int64 = -1
	incs := 0
	for i := 0; i < maxReads && incs < wantIncrements; i++ {
		n := Now()
		if n < prev {
			t.Fatalf("retrocede: %d < %d", n, prev)
		}
		if d := n - prev; d > 0 {
			incs++
			if minInc < 0 || d < minInc {
				minInc = d
			}
		}
		prev = n
	}
	if incs == 0 {
		t.Fatal("el reloj no avanzó")
	}
	// Resolución fina: el menor incremento positivo debe ser < 50 µs
	// (QPC ~100 ns; un reloj grueso de Windows daría ~0,5-15 ms).
	if minInc >= int64(50*time.Microsecond) {
		t.Fatalf("resolución gruesa: menor incremento %v", time.Duration(minInc))
	}
}

func TestSince(t *testing.T) {
	t0 := Now()
	time.Sleep(20 * time.Millisecond)
	if d := Since(t0); d < 15*time.Millisecond || d > 2*time.Second {
		t.Fatalf("Since = %v", d)
	}
}
