// Package version define la versión de NPerf y la regla de compatibilidad.
package version

import (
	"fmt"
	"strconv"
	"strings"
)

// Current es la versión de protocolo (handshake, User-Agent, historial) y
// fuente de la regla de compatibilidad.
var Current = "1.0.0"

// Build es la versión de compilación, solo para mostrar ("dev" o X.Y.Z).
// Se inyecta con -ldflags "-X nperf/internal/version.Build=X.Y.Z".
var Build = "dev"

// Display devuelve la versión de compilación para mostrar al usuario.
func Display() string { return Build }

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
