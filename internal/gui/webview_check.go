package gui

import (
	"errors"
	"fmt"
	"strings"
)

// WebView2DownloadURL es el enlace oficial (fijo) del instalador del runtime.
const WebView2DownloadURL = "https://go.microsoft.com/fwlink/p/?LinkId=2124703"

// ErrWebViewMissing indica que el runtime de WebView2 no está instalado o está bloqueado.
var ErrWebViewMissing = errors.New("falta Microsoft Edge WebView2 Runtime")

// VersionInstalled interpreta el valor "pv" del registro de EdgeUpdate: vacío o
// "0.0.0.0" significa no instalado.
func VersionInstalled(pv string) bool {
	pv = strings.TrimSpace(pv)
	return pv != "" && pv != "0.0.0.0"
}

// MissingRuntimeText es el texto del aviso cuando falta WebView2.
func MissingRuntimeText() string {
	return "NPerf necesita Microsoft Edge WebView2 Runtime y no está instalado (o está bloqueado por una política del equipo).\n\n" +
		"Descárguelo desde:\n" + WebView2DownloadURL + "\n\n" +
		"Mientras tanto puede usar nperf.exe desde la consola.\n\n" +
		"¿Abrir la página de descarga ahora?"
}

// StartupText elige el texto del cuadro de error para un fallo de arranque. Si es
// por falta de WebView2 devuelve offerDownload=true (el texto ya incluye la pregunta).
func StartupText(err error) (text string, offerDownload bool) {
	switch {
	case err == nil:
		return "", false
	case errors.Is(err, ErrWebViewMissing):
		return MissingRuntimeText(), true
	default:
		return "No se pudo iniciar NPerf:\n\n" + err.Error() + "\n\nPuede usar nperf.exe desde la consola.", false
	}
}

// RuntimeStatus es el resultado orientativo de buscar el runtime en el registro.
type RuntimeStatus int

const (
	RuntimeUnknown   RuntimeStatus = iota // error de lectura u otro caso no concluyente
	RuntimeInstalled                      // pv válido
	RuntimeMissing                        // claves ausentes o pv vacío/0.0.0.0
)

// CombineStatus resume varias sondas: una instalada basta; si no, cualquier
// desconocida impide afirmar que falta.
func CombineStatus(sts ...RuntimeStatus) RuntimeStatus {
	res := RuntimeMissing
	for _, s := range sts {
		if s == RuntimeInstalled {
			return RuntimeInstalled
		}
		if s == RuntimeUnknown {
			res = RuntimeUnknown
		}
	}
	return res
}

// WindowCreateError es el error cuando falla la creación de la ventana: solo si el
// registro dijo "no instalado" envuelve ErrWebViewMissing (ofrece la descarga).
func WindowCreateError(st RuntimeStatus) error {
	if st == RuntimeMissing {
		return fmt.Errorf("no se pudo crear la ventana: %w", ErrWebViewMissing)
	}
	return errors.New("no se pudo crear la ventana de WebView2 (revise el runtime o las políticas del equipo)")
}
