package gui

import "errors"

// ErrSaveCancelled indica que el usuario cerró el diálogo de guardar sin elegir ruta.
var ErrSaveCancelled = errors.New("guardado cancelado")
