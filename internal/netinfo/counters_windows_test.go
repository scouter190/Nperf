//go:build windows

package netinfo

import (
	"errors"
	"testing"
	"unsafe"
)

func TestMibIfRow2Layout(t *testing.T) {
	var r mibIfRow2
	// Tamaño y offsets calculados con el diseño de netioapi.h en amd64.
	checks := []struct {
		name      string
		got, want uintptr
	}{
		{"Sizeof", unsafe.Sizeof(r), 1352},
		{"OutQLen", unsafe.Offsetof(r.OutQLen), 1344},
		{"Tabla", unsafe.Sizeof(mibIfTable2{}), 8},
		{"InterfaceGuid", unsafe.Offsetof(r.InterfaceGuid), 12},
		{"OperStatus", unsafe.Offsetof(r.OperStatus), 1156},
		{"InOctets", unsafe.Offsetof(r.InOctets), 1208},
		{"OutOctets", unsafe.Offsetof(r.OutOctets), 1280},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %d, se esperaba %d", c.name, c.got, c.want)
		}
	}
}

func TestSystemCountersFreesOnce(t *testing.T) {
	frees := 0
	src := winCounters{
		get:  getIfTable2,
		free: func(p unsafe.Pointer) { frees++; freeMibTable(p) },
	}
	cs, err := src.Counters()
	if err != nil {
		t.Fatal(err)
	}
	if frees != 1 {
		t.Fatalf("FreeMibTable llamado %d veces", frees)
	}
	if len(cs) < 1 {
		t.Fatal("sin adaptadores")
	}
	withBytes := 0
	for _, c := range cs {
		if len(c.GUID) != 38 || c.GUID[0] != '{' || c.GUID[37] != '}' {
			t.Errorf("GUID inesperado %q", c.GUID)
		}
		if c.In > 0 || c.Out > 0 {
			withBytes++
		}
	}
	if withBytes < 1 {
		t.Error("ningún adaptador con contadores")
	}
}

func TestSystemCountersFreesOnError(t *testing.T) {
	frees := 0
	src := winCounters{
		get:  func() (unsafe.Pointer, error) { return nil, errors.New("x") },
		free: func(unsafe.Pointer) { frees++ },
	}
	if _, err := src.Counters(); err == nil {
		t.Fatal("se esperaba error")
	}
	if frees != 0 {
		t.Fatal("no debe liberar si get falla")
	}
}
