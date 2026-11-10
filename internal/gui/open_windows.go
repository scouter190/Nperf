package gui

import (
	"errors"
	"syscall"
	"unsafe"
)

func init() { OpenURL = shellOpen }

// shellOpen abre la URL con ShellExecuteW. Solo debe recibir la URL fija de App.
func shellOpen(url string) error {
	verb, _ := syscall.UTF16PtrFromString("open")
	u, err := syscall.UTF16PtrFromString(url)
	if err != nil {
		return err
	}
	r, _, _ := syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW").Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(u)), 0, 0, 1)
	if r <= 32 {
		return errors.New("no se pudo abrir la configuración de ubicación")
	}
	return nil
}
