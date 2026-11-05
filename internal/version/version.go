// Package version define la versión de NPerf y la regla de compatibilidad.
package version

import (
	"fmt"
	"strconv"
	"strings"
)

// Current es la versión semver de NPerf; única fuente de verdad.
const Current = "0.1.0"

// Major devuelve la versión mayor de una cadena semver "X.Y.Z".
func Major(v string) (int, error) {
	first, _, _ := strings.Cut(v, ".")
	n, err := strconv.Atoi(first)
	if err != nil || n < 0 || !strings.Contains(v, ".") {
		return 0, fmt.Errorf("versión inválida %q", v)
	}
	return n, nil
}

// Compatible indica si dos versiones comparten versión mayor.
// Las versiones menores o parches distintos son compatibles.
func Compatible(a, b string) (bool, error) {
	ma, err := Major(a)
	if err != nil {
		return false, err
	}
	mb, err := Major(b)
	if err != nil {
		return false, err
	}
	return ma == mb, nil
}
