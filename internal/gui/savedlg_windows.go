package gui

import (
	"errors"
	"sync/atomic"
	"syscall"
	"unsafe"
)

// ownerHWND es la ventana principal (la fija RunWindow) para que el diálogo sea modal.
var ownerHWND atomic.Uintptr

const (
	ofnOverwritePrompt = 0x2
	ofnPathMustExist   = 0x800
	ofnNoChangeDir     = 0x8
	ofnExplorer        = 0x80000
	saveBufChars       = 32768
)

// openFileName es OPENFILENAMEW (x64/x86 con los mismos campos; el compilador alinea).
type openFileName struct {
	StructSize    uint32
	Owner         uintptr
	Instance      uintptr
	Filter        *uint16
	CustomFilter  *uint16
	MaxCustFilter uint32
	FilterIndex   uint32
	File          *uint16
	MaxFile       uint32
	FileTitle     *uint16
	MaxFileTitle  uint32
	InitialDir    *uint16
	Title         *uint16
	Flags         uint32
	FileOffset    uint16
	FileExtension uint16
	DefExt        *uint16
	CustData      uintptr
	FnHook        uintptr
	TemplateName  *uint16
	PvReserved    uintptr
	DwReserved    uint32
	FlagsEx       uint32
}

var filters = map[string]struct{ name, pattern, ext string }{
	"csv":     {"CSV para Excel (*.csv)", "*.csv", "csv"},
	"csv-std": {"CSV estándar (*.csv)", "*.csv", "csv"},
	"json":    {"JSON (*.json)", "*.json", "json"},
	"html":    {"Informe HTML (*.html)", "*.html", "html"},
}

// SaveDialog abre el diálogo nativo "Guardar como" (GetSaveFileNameW) con el filtro del
// formato y el nombre sugerido. Debe llamarse desde el hilo de la UI (el puente de
// go-webview2 ya corre en él). Devuelve ErrSaveCancelled si el usuario cancela.
func SaveDialog(format, suggested string) (string, error) {
	f, ok := filters[format]
	if !ok {
		return "", errors.New("formato no permitido")
	}
	// Filtro: pares "nombre\0patrón\0" terminados en doble NUL.
	filter := utf16(f.name + "\x00" + f.pattern + "\x00")
	filter = append(filter, 0)
	buf := make([]uint16, saveBufChars)
	copy(buf, utf16(suggested)) // utf16 acaba en NUL; buf ya está a cero
	title, _ := syscall.UTF16PtrFromString("Guardar exportación de NPerf")
	def, _ := syscall.UTF16PtrFromString(f.ext)
	o := openFileName{
		Owner:  ownerHWND.Load(),
		Filter: &filter[0], FilterIndex: 1,
		File: &buf[0], MaxFile: saveBufChars,
		Title: title, DefExt: def,
		Flags: ofnOverwritePrompt | ofnPathMustExist | ofnNoChangeDir | ofnExplorer,
	}
	o.StructSize = uint32(unsafe.Sizeof(o))
	dll := syscall.NewLazyDLL("comdlg32.dll")
	r, _, _ := dll.NewProc("GetSaveFileNameW").Call(uintptr(unsafe.Pointer(&o)))
	if r == 0 {
		code, _, _ := dll.NewProc("CommDlgExtendedError").Call()
		if code == 0 {
			return "", ErrSaveCancelled
		}
		return "", errors.New("no se pudo abrir el diálogo de guardar")
	}
	return syscall.UTF16ToString(buf), nil
}

// utf16 convierte a UTF-16 conservando los NUL internos (a diferencia de UTF16FromString).
func utf16(s string) []uint16 {
	out := make([]uint16, 0, len(s)+1)
	for _, r := range s {
		if r < 0x10000 {
			out = append(out, uint16(r))
			continue
		}
		r -= 0x10000
		out = append(out, uint16(0xd800+(r>>10)), uint16(0xdc00+(r&0x3ff)))
	}
	return append(out, 0)
}
