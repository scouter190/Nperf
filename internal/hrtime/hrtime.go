// Package hrtime da marcas de tiempo monotónicas de alta resolución.
// El reloj de Go en Windows puede avanzar a saltos gruesos (~0,5-16 ms), lo que
// cuantiza a 0 los RTT de LAN; en Windows se usa QueryPerformanceCounter.
package hrtime

import "time"

// Since devuelve el tiempo transcurrido desde una marca t obtenida con Now.
func Since(t int64) time.Duration { return time.Duration(Now() - t) }
