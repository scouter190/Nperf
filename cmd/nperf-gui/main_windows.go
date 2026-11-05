// Comando nperf-gui: interfaz gráfica de NPerf (esqueleto; la ventana llega en T15).
package main

import (
	"syscall"
	"unsafe"
)

func main() {
	title, _ := syscall.UTF16PtrFromString("NPerf")
	text, _ := syscall.UTF16PtrFromString("NPerf GUI — en construcción")
	mb := syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")
	mb.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0)
}
