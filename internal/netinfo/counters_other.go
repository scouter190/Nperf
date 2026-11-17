//go:build !windows

package netinfo

import "errors"

type otherCounters struct{}

// NewSystemCounters devuelve una fuente que siempre falla: solo Windows está soportado.
func NewSystemCounters() CounterSource { return otherCounters{} }

func (otherCounters) Counters() ([]IfCounter, error) {
	return nil, errors.New("no soportado en este sistema")
}
