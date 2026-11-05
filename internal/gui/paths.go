package gui

import "path/filepath"

// Paths resuelve las rutas de la GUI a partir de os.UserConfigDir(): el directorio que se
// pasa a NewService (las funciones de internal/config ya agregan "NPerf", igual que la CLI)
// y la carpeta de datos de WebView2.
func Paths(userConfigDir string) (configDir, webviewDir string) {
	return userConfigDir, filepath.Join(userConfigDir, "NPerf", "webview2")
}
