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

const (
	mbYesNo = 0x00000004
	idYes   = 6
)

func messageBox(title, text string, flags uintptr) uintptr {
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(text)
	r, _, _ := syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), flags)
	return r
}

// ShowStartupError muestra el error de arranque en español; si falta WebView2
// ofrece abrir la descarga oficial (solo la URL fija).
func ShowStartupError(err error) {
	text, offer := StartupText(err)
	if !offer {
		messageBox("NPerf", text, mbOK|mbIconError)
		return
	}
	if messageBox("NPerf", text, mbYesNo|mbIconError) == idYes {
		verb, _ := syscall.UTF16PtrFromString("open")
		u, _ := syscall.UTF16PtrFromString(WebView2DownloadURL)
		syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW").Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(u)), 0, 0, 1)
	}
}

// ShowError muestra un error en un cuadro de mensaje (la app no tiene consola).
func ShowError(text string) { messageBox("NPerf", text, mbOK|mbIconError) }

// ShowInfo muestra un aviso en un cuadro de mensaje.
func ShowInfo(text string) { messageBox("NPerf", text, mbOK|mbIconInfo) }
