package gui

import "sync"

// Textos del menú de la bandeja.
const (
	TrayTitle    = "NPerf"
	TrayOpenText = "Abrir"
	TrayQuitText = "Salir"
	trayCmdOpen  = 1
	trayCmdQuit  = 2
)

// TrayEvent es una acción del usuario sobre el icono de la bandeja.
type TrayEvent int

const (
	TrayDoubleClick TrayEvent = iota + 1 // doble clic: Abrir
	TrayContextMenu                      // clic derecho: mostrar menú
)

// TrayActions son las acciones sobre la ventana; se ejecutan en el hilo de la UI.
type TrayActions struct {
	Show func() // muestra y trae al frente la ventana
	Hide func() // oculta la ventana
	Quit func() // termina el bucle de la UI (la app se detiene limpiamente)
}

// TrayController es la lógica de la bandeja, independiente de Win32.
type TrayController struct {
	act TrayActions

	mu      sync.Mutex
	quiting bool
}

// NewTrayController crea el controlador con las acciones dadas.
func NewTrayController(a TrayActions) *TrayController { return &TrayController{act: a} }

// OnClose se llama al cerrar la ventana. Devuelve true si se gestionó ocultándola
// (la app sigue en la bandeja) y false si se debe dejar cerrar (ya se está saliendo).
func (c *TrayController) OnClose() bool {
	c.mu.Lock()
	q := c.quiting
	c.mu.Unlock()
	if q {
		return false
	}
	c.act.Hide()
	return true
}

// OnIcon procesa un evento del icono; devuelve true si hay que mostrar el menú.
func (c *TrayController) OnIcon(e TrayEvent) (showMenu bool) {
	switch e {
	case TrayDoubleClick:
		c.act.Show()
	case TrayContextMenu:
		return true
	}
	return false
}

// OnCommand procesa una opción del menú (trayCmdOpen / trayCmdQuit).
func (c *TrayController) OnCommand(id int) {
	switch id {
	case trayCmdOpen:
		c.act.Show()
	case trayCmdQuit:
		c.Quit()
	}
}

// Quit marca la salida (idempotente) y detiene el bucle de la UI.
func (c *TrayController) Quit() {
	c.mu.Lock()
	first := !c.quiting
	c.quiting = true
	c.mu.Unlock()
	if first {
		c.act.Quit()
	}
}
