// Package tcpinfo lee el contador de retransmisiones de un socket TCP, por SO.
//
// Solo tiene sentido en quien ENVÍA. Se lee al terminar el envío de cada flujo; como las
// conexiones de datos son nuevas, el contador acumulado de la conexión equivale a lo
// retransmitido durante la prueba (no se resta un valor inicial).
package tcpinfo

import "errors"

// ErrUnavailable indica que el SO o la versión no entrega retransmisiones (se muestra
// como "no disponible").
var ErrUnavailable = errors.New("retransmisiones no disponibles")
