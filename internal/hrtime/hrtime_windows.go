package hrtime

import (
	"syscall"
	"unsafe"
)

var (
	k32     = syscall.NewLazyDLL("kernel32.dll")
	procQPC = k32.NewProc("QueryPerformanceCounter")
	procQPF = k32.NewProc("QueryPerformanceFrequency")
	freq    = queryFreq()
)

func queryFreq() int64 {
	var f int64
	r, _, _ := procQPF.Call(uintptr(unsafe.Pointer(&f)))
	if r == 0 || f <= 0 {
		return 0
	}
	return f
}

// Now devuelve ns monotónicos (origen arbitrario, común al proceso).
func Now() int64 {
	var c int64
	r, _, _ := procQPC.Call(uintptr(unsafe.Pointer(&c)))
	if r == 0 || freq == 0 {
		return fallback()
	}
	// Sin overflow: c/f segundos enteros más el resto (< f) escalado a ns.
	return c/freq*1e9 + c%freq*1e9/freq
}
