package gui

import "testing"

func newTestTray() (*TrayController, *[]string) {
	var log []string
	c := NewTrayController(TrayActions{
		Show:        func() { log = append(log, "show") },
		Hide:        func() { log = append(log, "hide") },
		Quit:        func() { log = append(log, "quit") },
		StopMonitor: func() { log = append(log, "stopmon") },
	})
	return c, &log
}

func TestTrayCloseHides(t *testing.T) {
	c, log := newTestTray()
	if !c.OnClose() || len(*log) != 2 || (*log)[0] != "hide" || (*log)[1] != "stopmon" {
		t.Fatalf("cerrar debe ocultar: %v", *log)
	}
}

func TestTrayIconAndMenu(t *testing.T) {
	c, log := newTestTray()
	if c.OnIcon(TrayDoubleClick) {
		t.Fatal("doble clic no muestra menú")
	}
	if !c.OnIcon(TrayContextMenu) {
		t.Fatal("clic derecho debe mostrar menú")
	}
	c.OnCommand(trayCmdOpen)
	if len(*log) != 2 || (*log)[0] != "show" || (*log)[1] != "show" {
		t.Fatalf("abrir: %v", *log)
	}
}

func TestTrayQuitOnceAndCloseAfterwards(t *testing.T) {
	c, log := newTestTray()
	c.OnCommand(trayCmdQuit)
	c.OnCommand(trayCmdQuit)
	if len(*log) != 2 || (*log)[0] != "stopmon" || (*log)[1] != "quit" {
		t.Fatalf("salir una vez: %v", *log)
	}
	if c.OnClose() {
		t.Fatal("al salir, cerrar no debe ocultar")
	}
}
