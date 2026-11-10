package netinfo

import "testing"

func TestLinkSpeedBps(t *testing.T) {
	if linkSpeedBps(^uint64(0)) != 0 || linkSpeedBps(0) != 0 || linkSpeedBps(1e9) != 1e9 {
		t.Fatal("conversión incorrecta")
	}
}
