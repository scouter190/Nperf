package probe

import (
	"fmt"
	"strconv"
	"strings"
)

// MaxPorts es el máximo de puertos por ejecución.
const MaxPorts = 32

// DefaultPorts es la lista por defecto (8291 = Winbox).
var DefaultPorts = []int{21, 22, 23, 53, 80, 443, 445, 3389, 8080, 8291}

// ParsePorts interpreta "22,80,8000-8010": 1-65535, sin duplicados, máx. 32.
func ParsePorts(s string) ([]int, error) {
	if strings.TrimSpace(s) == "" {
		return nil, fmt.Errorf("lista de puertos vacía")
	}
	seen := map[int]bool{}
	var out []int
	add := func(p int) error {
		if p < 1 || p > 65535 {
			return fmt.Errorf("puerto %d fuera de rango (1-65535)", p)
		}
		if seen[p] {
			return fmt.Errorf("puerto %d repetido", p)
		}
		if len(out) >= MaxPorts {
			return fmt.Errorf("demasiados puertos (máx. %d)", MaxPorts)
		}
		seen[p] = true
		out = append(out, p)
		return nil
	}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		lo, hi, isRange := strings.Cut(part, "-")
		a, err := strconv.Atoi(strings.TrimSpace(lo))
		if err != nil {
			return nil, fmt.Errorf("puerto no válido: %q", part)
		}
		b := a
		if isRange {
			if b, err = strconv.Atoi(strings.TrimSpace(hi)); err != nil {
				return nil, fmt.Errorf("rango no válido: %q", part)
			}
			if b < a {
				return nil, fmt.Errorf("rango invertido: %q", part)
			}
			if a < 1 || b > 65535 {
				return nil, fmt.Errorf("rango %q fuera de 1-65535", part)
			}
			if b-a+1 > MaxPorts {
				return nil, fmt.Errorf("demasiados puertos (máx. %d)", MaxPorts)
			}
		}
		for p := a; p <= b; p++ {
			if err := add(p); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}
