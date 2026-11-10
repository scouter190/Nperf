package netinfo

// linkSpeedBps convierte la velocidad de GetAdaptersAddresses: el valor
// desconocido (^uint64(0)) pasa a 0 (no disponible).
func linkSpeedBps(v uint64) uint64 {
	if v == ^uint64(0) {
		return 0
	}
	return v
}
