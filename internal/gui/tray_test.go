package gui

import "testing"

func newTestTray() (*TrayController, *[]string) {
	var log []string
	c := NewTrayController(TrayActions{
		Show: func() { log = append(log, "show") },
		Hide: func() { log = append(log, "hide") },
		Quit: func() { log = append(log, "quit") },
	})
	return c, &log
}

func TestTrayCloseHides(t *testing.T) {
	c, log := newTestTray()
	if !c.OnClose() || len(*log) != 1 || (*log)[0] != "hide" {
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
	if len(*log) != 1 || (*log)[0] != "quit" {
		t.Fatalf("salir una vez: %v", *log)
	}
	if c.OnClose() {
		t.Fatal("al salir, cerrar no debe ocultar")
	}
}
