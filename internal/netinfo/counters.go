package netinfo

// IfCounter son los contadores acumulados de un adaptador (GetIfTable2 en Windows).
type IfCounter struct {
	GUID string // "{...}", igual que RawAdapter.GUID
	In   uint64 // octetos recibidos
	Out  uint64 // octetos enviados
	Up   bool   // enlace operativo
}

// CounterSource es la fuente inyectable de contadores de interfaz.
type CounterSource interface {
	Counters() ([]IfCounter, error)
}
