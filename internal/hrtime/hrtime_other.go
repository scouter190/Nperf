//go:build !windows

package hrtime

// Now devuelve ns monotónicos desde un origen fijo del proceso.
func Now() int64 { return fallback() }
