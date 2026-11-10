package netinfo

import (
	"strconv"
	"strings"
)

// FormatBps formatea bits por segundo: "100 Mbps", "1 Gbps", "2.5 Gbps".
func FormatBps(bps uint64) string {
	units := []struct {
		div  uint64
		name string
	}{{1_000_000_000, "Gbps"}, {1_000_000, "Mbps"}, {1_000, "Kbps"}}
	for _, u := range units {
		if bps >= u.div {
			v := float64(bps) / float64(u.div)
			s := strconv.FormatFloat(v, 'f', 1, 64)
			s = strings.TrimSuffix(s, ".0")
			return s + " " + u.name
		}
	}
	return strconv.FormatUint(bps, 10) + " bps"
}
