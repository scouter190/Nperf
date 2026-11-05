// Package ui contiene la interfaz HTML/CSS/JS embebida (sin frameworks ni CDN).
package ui

import (
	"embed"
	"io/fs"
)

//go:embed web/index.html web/app.css web/app.js web/mock.js
var files embed.FS

// Files devuelve la raíz de los archivos web (index.html, app.css, app.js, mock.js).
// mock.js es solo para previsualizar y probar: index.html no lo carga.
func Files() fs.FS {
	sub, err := fs.Sub(files, "web")
	if err != nil {
		panic(err)
	}
	return sub
}
