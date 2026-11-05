package gui

import (
	"syscall"
	"unsafe"
)

const (
	mbOK        = 0x00000000
	mbIconError = 0x00000010
	mbIconInfo  = 0x00000040
)

func messageBox(title, text string, flags uintptr) {
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(text)
	syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), flags)
}

// ShowError muestra un error en un cuadro de mensaje (la app no tiene consola).
func ShowError(text string) { messageBox("NPerf", text, mbOK|mbIconError) }

// ShowInfo muestra un aviso en un cuadro de mensaje.
func ShowInfo(text string) { messageBox("NPerf", text, mbOK|mbIconInfo) }
