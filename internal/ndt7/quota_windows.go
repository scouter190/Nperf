package ndt7

import (
	"errors"

	"golang.org/x/sys/windows"
)

// isTransient reconoce los errores de Windows que aparecen mientras otro
// proceso o goroutine tiene el archivo abierto o pendiente de borrar.
func isTransient(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_ACCESS_DENIED) ||
		errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}
