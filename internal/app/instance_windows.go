//go:build windows

package app

import (
	"fmt"
	"syscall"
	"unsafe"
)

const errorAlreadyExists = 183

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procCreateMutexW = kernel32.NewProc("CreateMutexW")
)

// Acquire toma un mutex con nombre en el espacio de nombres de la sesión del
// usuario (prefijo Local\, sin privilegios de administrador). Si ya existe,
// devuelve ErrAlreadyRunning. release cierra el handle.
func Acquire(name string) (release func(), err error) {
	p, err := syscall.UTF16PtrFromString(`Local\` + name)
	if err != nil {
		return nil, err
	}
	h, _, callErr := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(p)))
	if h == 0 {
		return nil, fmt.Errorf("CreateMutexW: %w", callErr)
	}
	if errno, ok := callErr.(syscall.Errno); ok && errno == errorAlreadyExists {
		syscall.CloseHandle(syscall.Handle(h))
		return nil, ErrAlreadyRunning
	}
	return func() { syscall.CloseHandle(syscall.Handle(h)) }, nil
}
