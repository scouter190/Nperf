// Package app agrupa utilidades de la aplicación de escritorio.
package app

import "errors"

// ErrAlreadyRunning indica que ya hay otra instancia con el mismo nombre.
var ErrAlreadyRunning = errors.New("ya hay otra instancia de NPerf en ejecución")
