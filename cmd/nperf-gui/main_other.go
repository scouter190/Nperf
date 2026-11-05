//go:build !windows

// Comando nperf-gui: solo Windows; en otros SO compila y sale con un mensaje.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "nperf-gui solo está disponible en Windows")
	os.Exit(1)
}
