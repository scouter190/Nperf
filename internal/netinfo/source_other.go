//go:build !windows

package netinfo

import "errors"

type otherSource struct{}

// NewSystemSource devuelve una fuente que siempre falla: solo Windows está soportado.
func NewSystemSource(wifi func(guid string) RawWiFi) Source { return otherSource{} }

func (otherSource) Adapters() ([]RawAdapter, error) {
	return nil, errors.New("no soportado en este sistema")
}

func (otherSource) WiFi(string) (RawWiFi, error) {
	return RawWiFi{}, errors.New("no soportado en este sistema")
}

// System devuelve la fuente "no soportado" fuera de Windows.
func System() Source { return otherSource{} }
