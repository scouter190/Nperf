package client

import (
	"errors"
	"testing"
	"time"
)

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func ms(n int) time.Duration { return time.Duration(n) * time.Millisecond }

func TestProberOnePerSecondNeverTwoInFlight(t *testing.T) {
	var sent []int
	maxInflight := 0
	p := NewProber(t0, 5, func(seq int) error {
		sent = append(sent, seq)
		return nil
	})
	// Sin contestar: cada sonda expira a 1 s y la siguiente sale en el mismo instante.
	timeouts := 0
	for now := t0; now.Before(t0.Add(8 * time.Second)); now = now.Add(100 * time.Millisecond) {
		res, err := p.Tick(now)
		if err != nil {
			t.Fatal(err)
		}
		timeouts += len(res)
		if n := len(sent) - timeouts; n > maxInflight {
			maxInflight = n
		}
	}
	if len(sent) != 5 || timeouts != 5 || maxInflight != 1 {
		t.Fatalf("sent=%v timeouts=%d max=%d", sent, timeouts, maxInflight)
	}
	if !p.Done() {
		t.Fatal("debía terminar")
	}
}

func TestProberAnsweredRate(t *testing.T) {
	var sentAt []time.Time
	var now time.Time
	p := NewProber(t0, 3, func(int) error { sentAt = append(sentAt, now); return nil })
	for now = t0; now.Before(t0.Add(4 * time.Second)); now = now.Add(50 * time.Millisecond) {
		p.Tick(now)
		if p.InFlight() {
			if r, ok := p.Pong(p.seq, now.Add(ms(10))); !ok || r.RTT != ms(10) {
				t.Fatalf("pong %v %v", r, ok)
			}
		}
	}
	if len(sentAt) != 3 {
		t.Fatalf("sondas %d", len(sentAt))
	}
	for i, s := range sentAt {
		if s.Sub(t0) != time.Duration(i)*time.Second {
			t.Fatalf("sonda %d a %v", i, s.Sub(t0))
		}
	}
}

func TestProberPongRules(t *testing.T) {
	tests := []struct {
		name string
		seq  int
		at   time.Duration
		ok   bool
	}{
		{"valido", 0, ms(30), true},
		{"otro seq", 1, ms(30), false},
		{"tardio 1 s", 0, time.Second, false},
		{"tardio", 0, 1500 * time.Millisecond, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := NewProber(t0, 3, func(int) error { return nil })
			p.Tick(t0)
			_, ok := p.Pong(tc.seq, t0.Add(tc.at))
			if ok != tc.ok {
				t.Fatalf("ok=%v", ok)
			}
		})
	}
}

func TestProberTimeoutAndLatePong(t *testing.T) {
	p := NewProber(t0, 3, func(int) error { return nil })
	p.Tick(t0)
	res, _ := p.Tick(t0.Add(time.Second))
	if len(res) != 1 || res[0].OK || res[0].Seq != 0 {
		t.Fatalf("res=%v", res)
	}
	// La sonda 1 está en vuelo; el pong de la 0 se ignora.
	if _, ok := p.Pong(0, t0.Add(1100*time.Millisecond)); ok {
		t.Fatal("pong tardío aceptado")
	}
	if !p.InFlight() {
		t.Fatal("la sonda 1 debía seguir en vuelo")
	}
}

func TestProberSendError(t *testing.T) {
	p := NewProber(t0, 3, func(int) error { return errors.New("x") })
	if _, err := p.Tick(t0); err == nil {
		t.Fatal("esperaba error")
	}
	if !p.Done() {
		t.Fatal("tras error no debe enviar más")
	}
}

func TestNewLoadedRTT(t *testing.T) {
	tests := []struct {
		name string
		in   []time.Duration
		want *LoadedRTT
	}{
		{"vacio", nil, nil},
		{"una", []time.Duration{ms(5)}, &LoadedRTT{ms(5), ms(5), ms(5), ms(5), 1, 4}},
		{"impar", []time.Duration{ms(30), ms(10), ms(20)}, &LoadedRTT{ms(10), ms(20), ms(20), ms(30), 3, 4}},
		{"par", []time.Duration{ms(40), ms(10), ms(20), ms(30)}, &LoadedRTT{ms(10), ms(25), ms(25), ms(40), 4, 4}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NewLoadedRTT(tc.in, 4)
			if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
				t.Fatalf("got %+v want %+v", got, tc.want)
			}
		})
	}
}

func TestBufferbloatNote(t *testing.T) {
	init := ms(10)
	tests := []struct {
		name   string
		median time.Duration
		l      *LoadedRTT
		want   string
	}{
		{"+100 si", ms(110), &LoadedRTT{Median: ms(110), Samples: 3}, "la latencia sube 100 ms con carga en ida: colas llenas en el enlace (bufferbloat)"},
		{"+99 no", ms(109), &LoadedRTT{Median: ms(109), Samples: 3}, ""},
		{"sin datos", 0, nil, ""},
		{"sin muestras", 0, &LoadedRTT{}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := BufferbloatNote(init, tc.l, "ida"); got != tc.want {
				t.Fatalf("got %q", got)
			}
		})
	}
	if BufferbloatNote(0, &LoadedRTT{Median: ms(500), Samples: 1}, "ida") != "" {
		t.Fatal("sin RTT inicial no hay nota")
	}
}

func TestProberSendDoesNotBlockPong(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	p := NewProber(t0, 3, func(int) error {
		close(entered)
		<-release
		return nil
	})
	done := make(chan struct{})
	go func() { p.Tick(t0); close(done) }()
	<-entered
	got := make(chan bool)
	go func() { _, ok := p.Pong(0, t0.Add(ms(5))); got <- ok }()
	select {
	case ok := <-got:
		if !ok {
			t.Fatal("pong rechazado")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Pong bloqueado por send")
	}
	close(release)
	<-done
}
