package gui

import (
	"encoding/json"
	"errors"
	"fmt"
	"runtime"

	webview2 "github.com/jchv/go-webview2"
)

func init() {
	// WebView2 (COM STA) y su bucle de mensajes deben vivir en un único hilo del SO.
	runtime.LockOSThread()
}

// RunWindow abre la ventana "NPerf" con la UI en url y enlaza window.nperfCall a
// call (lista blanca en Bridge.Call). Bloquea hasta que se cierra la ventana; al
// cerrarla se oculta en la bandeja; solo "Salir" del menú termina la app.
// dataDir es la carpeta de datos de WebView2.
//
// go-webview2 no expone NavigationStarting; la protección del puente es el secreto por
// sesión + CSP sin enlaces externos. call recibe el secreto como primer argumento de
// nperfCall y lo verifica (Bridge.Call): una página externa no puede leerlo.
func RunWindow(url, dataDir string, call func(secret, name string, args ...json.RawMessage) (any, error)) error {
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		DataPath: dataDir,
		WindowOptions: webview2.WindowOptions{
			Title: "NPerf", Width: 1000, Height: 640, Center: true,
		},
	})
	if w == nil {
		return errors.New("no se pudo iniciar WebView2: instale el runtime de WebView2 de Microsoft")
	}
	defer w.Destroy()
	w.SetSize(1000, 640, webview2.HintNone)
	w.SetSize(720, 480, webview2.HintMin)
	// Un panic en un método no debe tumbar la app: llega a la UI como error.
	safe := func(secret, name string, args ...json.RawMessage) (res any, err error) {
		defer func() {
			if r := recover(); r != nil {
				res, err = nil, fmt.Errorf("error interno: %v", r)
			}
		}()
		return call(secret, name, args...)
	}
	if err := w.Bind("nperfCall", safe); err != nil {
		return err
	}
	// Bandeja: cerrar oculta la ventana; "Salir" termina el bucle y Run vuelve (el
	// llamador detiene server y anuncio al salir). Si la bandeja falla, la ventana
	// funciona como antes (cerrar = salir).
	ctl := NewTrayController(TrayActions{Quit: w.Terminate})
	if tr, err := installTray(uintptr(w.Window()), ctl); err == nil {
		ctl.act.Show, ctl.act.Hide = tr.showMain, tr.hideMain
		defer tr.remove()
	}
	w.Navigate(url)
	w.Run()
	return nil
}
