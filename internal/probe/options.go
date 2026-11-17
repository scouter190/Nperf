package probe

import (
	"fmt"
	"time"
)

// Límites de las opciones (spec 004).
const (
	MinCount, MaxCount     = 1, 100
	MinInterval            = 200 * time.Millisecond
	MinTimeout, MaxTimeout = 200 * time.Millisecond, 5 * time.Second
	MinHops, MaxHopsLimit  = 1, 64
	DefaultCount           = 10
	DefaultInterval        = 500 * time.Millisecond
	DefaultTimeout         = time.Second
	DefaultMaxHops         = 30
	DefaultPortsTimeout    = 1500 * time.Millisecond
	PayloadSize            = 32
)

// PingOptions son las opciones de ping.
type PingOptions struct {
	Count    int
	Interval time.Duration
	Timeout  time.Duration
}

// DefaultPing devuelve los valores por defecto.
func DefaultPing() PingOptions {
	return PingOptions{Count: DefaultCount, Interval: DefaultInterval, Timeout: DefaultTimeout}
}

// Validate comprueba los límites.
func (o PingOptions) Validate() error {
	if o.Count < MinCount || o.Count > MaxCount {
		return fmt.Errorf("el número de ecos debe estar entre %d y %d", MinCount, MaxCount)
	}
	if o.Interval < MinInterval {
		return fmt.Errorf("el intervalo mínimo es %d ms", MinInterval.Milliseconds())
	}
	return checkTimeout(o.Timeout)
}

// TraceOptions son las opciones de traceroute.
type TraceOptions struct {
	MaxHops int
	Timeout time.Duration
}

// DefaultTrace devuelve los valores por defecto.
func DefaultTrace() TraceOptions {
	return TraceOptions{MaxHops: DefaultMaxHops, Timeout: DefaultTimeout}
}

// Validate comprueba los límites.
func (o TraceOptions) Validate() error {
	if o.MaxHops < MinHops || o.MaxHops > MaxHopsLimit {
		return fmt.Errorf("el máximo de saltos debe estar entre %d y %d", MinHops, MaxHopsLimit)
	}
	return checkTimeout(o.Timeout)
}

// PortsOptions son las opciones de la prueba de puertos.
type PortsOptions struct {
	Ports   []int
	Timeout time.Duration
}

// DefaultPortsOptions usa la lista por defecto.
func DefaultPortsOptions() PortsOptions {
	return PortsOptions{Ports: append([]int(nil), DefaultPorts...), Timeout: DefaultPortsTimeout}
}

// Validate comprueba los límites.
func (o PortsOptions) Validate() error {
	if len(o.Ports) == 0 || len(o.Ports) > MaxPorts {
		return fmt.Errorf("debe indicar entre 1 y %d puertos", MaxPorts)
	}
	for _, p := range o.Ports {
		if p < 1 || p > 65535 {
			return fmt.Errorf("puerto %d fuera de rango (1-65535)", p)
		}
	}
	return checkTimeout(o.Timeout)
}

func checkTimeout(d time.Duration) error {
	if d < MinTimeout || d > MaxTimeout {
		return fmt.Errorf("el timeout debe estar entre %.1f y %.0f s", MinTimeout.Seconds(), MaxTimeout.Seconds())
	}
	return nil
}
