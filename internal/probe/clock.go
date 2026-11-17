package probe

import (
	"context"
	"sync"
	"time"
)

// Clock abstrae el tiempo para poder probar el ritmo sin esperas reales.
type Clock interface {
	Now() time.Time
	// Sleep espera d o hasta que ctx se cancele (devuelve ctx.Err()).
	Sleep(ctx context.Context, d time.Duration) error
}

// RealClock usa el reloj del sistema.
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

func (RealClock) Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// FakeClock avanza solo con Sleep y registra las esperas pedidas.
type FakeClock struct {
	mu     sync.Mutex
	now    time.Time
	sleeps []time.Duration
}

// NewFakeClock crea un reloj falso en el instante start.
func NewFakeClock(start time.Time) *FakeClock { return &FakeClock{now: start} }

func (f *FakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *FakeClock) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if d > 0 {
		f.now = f.now.Add(d)
	}
	f.sleeps = append(f.sleeps, d)
	return nil
}

// Sleeps devuelve una copia de las esperas registradas.
func (f *FakeClock) Sleeps() []time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]time.Duration(nil), f.sleeps...)
}
