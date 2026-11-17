package meter

import (
	"testing"
	"time"
)

var sT0 = time.Unix(1000, 0)

func ms(n int) time.Time { return sT0.Add(time.Duration(n) * time.Millisecond) }

func drain(ch chan Sample) []Sample {
	var r []Sample
	for {
		select {
		case s := <-ch:
			r = append(r, s)
		default:
			return r
		}
	}
}

func TestSeriesTCP(t *testing.T) {
	tests := []struct {
		name   string
		d      time.Duration
		end    int // ms del Finish
		wantN  int
		lastMs time.Duration
	}{
		{"3 s", 3 * time.Second, 3000, 3, time.Second},
		{"3 s fin tardio", 3 * time.Second, 3040, 3, 1040 * time.Millisecond},
		{"2.5 s", 2500 * time.Millisecond, 2500, 3, 500 * time.Millisecond},
		{"1 s", time.Second, 1000, 1, time.Second},
		{"0.2 s", 200 * time.Millisecond, 200, 1, 200 * time.Millisecond},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ch := make(chan Sample, MaxSamples)
			s := NewSeries(tc.d, ch)
			var total int64
			for ms_ := 0; ms_ < tc.end; ms_ += 10 {
				s.Add(ms(ms_), 100)
				total += 100
			}
			s.Finish(ms(tc.end))
			got := drain(ch)
			if len(got) != tc.wantN || s.Len() != tc.wantN {
				t.Fatalf("muestras = %d, want %d", len(got), tc.wantN)
			}
			var sum int64
			var dur time.Duration
			for i, g := range got {
				if g.Index != i {
					t.Errorf("index %d = %d", i, g.Index)
				}
				if g.Start != dur {
					t.Errorf("start %d = %v, want %v", i, g.Start, dur)
				}
				sum += g.Bytes
				dur += g.Duration
			}
			if sum != total {
				t.Errorf("suma = %d, want %d", sum, total)
			}
			if got[len(got)-1].Duration != tc.lastMs {
				t.Errorf("ultima duracion = %v, want %v", got[len(got)-1].Duration, tc.lastMs)
			}
			if dur != time.Duration(tc.end)*time.Millisecond {
				t.Errorf("duracion total = %v", dur)
			}
		})
	}
}

func TestSeriesStartsAtFirstByteAndLateTick(t *testing.T) {
	ch := make(chan Sample, 10)
	s := NewSeries(3*time.Second, ch)
	s.Observe(ms(500)) // antes del primer byte: nada
	s.Add(ms(1000), 10)
	s.Observe(ms(2050)) // borde nominal 2000, aviso tardío
	s.Add(ms(2500), 5)
	s.Finish(ms(4000))
	got := drain(ch)
	if len(got) != 3 {
		t.Fatalf("n = %d", len(got))
	}
	if got[0].Duration != 1050*time.Millisecond || got[0].Bytes != 10 {
		t.Errorf("0: %+v", got[0])
	}
	if got[1].Start != 1050*time.Millisecond || got[1].Bytes != 5 {
		t.Errorf("1: %+v", got[1])
	}
	if got[2].Bytes != 0 || got[2].Duration != time.Second {
		t.Errorf("2: %+v", got[2])
	}
}

func TestSeriesGapEmptyWindows(t *testing.T) {
	ch := make(chan Sample, 10)
	s := NewSeries(5*time.Second, ch)
	s.Add(ms(0), 7)
	s.Add(ms(3500), 3) // cruza bordes 1000, 2000 y 3000
	s.Finish(ms(5000))
	got := drain(ch)
	if len(got) != 5 {
		t.Fatalf("n = %d", len(got))
	}
	want := []int64{7, 0, 0, 3, 0}
	var dur time.Duration
	for i, g := range got {
		if g.Bytes != want[i] {
			t.Errorf("bytes[%d] = %d, want %d", i, g.Bytes, want[i])
		}
		dur += g.Duration
	}
	if dur != 5*time.Second {
		t.Errorf("duracion = %v", dur)
	}
}

func TestSeriesNoDataNoSamples(t *testing.T) {
	ch := make(chan Sample, 4)
	s := NewSeries(3*time.Second, ch)
	s.Finish(ms(3000))
	if got := drain(ch); len(got) != 0 {
		t.Fatalf("n = %d", len(got))
	}
}

func TestSeriesFinishEarlyAndIdempotent(t *testing.T) {
	ch := make(chan Sample, 10)
	s := NewSeries(3*time.Second, ch)
	s.Add(ms(0), 1)
	s.Finish(ms(300))
	s.Finish(ms(400))
	got := drain(ch)
	if len(got) != 3 || got[0].Duration != 0 || got[2].Duration != 300*time.Millisecond {
		t.Fatalf("%+v", got)
	}
}

func TestSeriesNonBlocking(t *testing.T) {
	ch := make(chan Sample, 1)
	s := NewSeries(3*time.Second, ch)
	s.Add(ms(0), 1)
	s.Finish(ms(3000))
	if s.Dropped() != 2 || len(ch) != 1 {
		t.Fatalf("dropped = %d len = %d", s.Dropped(), len(ch))
	}
	NewSeries(time.Second, nil).Add(ms(0), 1) // out nil no entra en pánico
}

type sPkt struct {
	at      int
	seq     uint64
	transit time.Duration
}

func TestSeriesUDP(t *testing.T) {
	tests := []struct {
		name     string
		d        time.Duration
		end      int
		pkts     []sPkt
		wantRecv []int64
		wantExp  []int64
		wantLost []int64
	}{
		{
			name: "sin perdida", d: 2 * time.Second, end: 2000,
			pkts:     []sPkt{{0, 0, 0}, {500, 1, 0}, {1000, 2, 0}, {1500, 3, 0}},
			wantRecv: []int64{2, 2}, wantExp: []int64{2, 2}, wantLost: []int64{0, 0},
		},
		{
			name: "perdida en un intervalo", d: 3 * time.Second, end: 3000,
			pkts:     []sPkt{{0, 0, 0}, {500, 1, 0}, {1000, 2, 0}, {1500, 5, 0}, {2000, 6, 0}},
			wantRecv: []int64{2, 2, 1}, wantExp: []int64{2, 4, 1}, wantLost: []int64{0, 2, 0},
		},
		{
			name: "desordenados entre intervalos",
			d:    2 * time.Second, end: 2000,
			// seq 2 llega en el intervalo 1 tras 3; el 0 espera 2 de 0..3 = 4
			pkts:     []sPkt{{0, 0, 0}, {500, 1, 0}, {900, 3, 0}, {1100, 2, 0}},
			wantRecv: []int64{3, 1}, wantExp: []int64{4, 0}, wantLost: []int64{1, 0},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ch := make(chan Sample, 10)
			s := NewSeries(tc.d, ch)
			for _, p := range tc.pkts {
				s.Packet(ms(p.at), 1200, p.seq, p.transit)
			}
			s.Finish(ms(tc.end))
			got := drain(ch)
			if len(got) != len(tc.wantRecv) {
				t.Fatalf("n = %d", len(got))
			}
			for i, g := range got {
				if !g.UDP || g.Received != tc.wantRecv[i] || g.Expected != tc.wantExp[i] || g.Lost != tc.wantLost[i] {
					t.Errorf("[%d] = %+v, want recv %d exp %d lost %d", i, g, tc.wantRecv[i], tc.wantExp[i], tc.wantLost[i])
				}
				if g.Bytes != g.Received*1200 {
					t.Errorf("[%d] bytes = %d", i, g.Bytes)
				}
			}
		})
	}
}

func TestSeriesUDPLossPercentAndJitter(t *testing.T) {
	ch := make(chan Sample, 4)
	s := NewSeries(2*time.Second, ch)
	s.Packet(ms(0), 100, 0, 0)
	s.Packet(ms(100), 100, 1, 16*time.Millisecond) // |D| = 16 ms -> J = 1 ms
	s.Packet(ms(1100), 100, 4, 16*time.Millisecond)
	s.Finish(ms(2000))
	got := drain(ch)
	if got[0].Jitter != time.Millisecond {
		t.Errorf("jitter 0 = %v", got[0].Jitter)
	}
	if want := 1 * time.Millisecond * 15 / 16; got[1].Jitter != want {
		t.Errorf("jitter 1 = %v, want %v", got[1].Jitter, want)
	}
	if got[1].Expected != 3 || got[1].Lost != 2 || got[1].LossPercent() < 66.6 || got[1].LossPercent() > 66.7 {
		t.Errorf("%+v %.2f", got[1], got[1].LossPercent())
	}
	if (Sample{}).LossPercent() != 0 {
		t.Error("loss sin esperados")
	}
}

func TestSampleMbps(t *testing.T) {
	s := Sample{Bytes: 125000, Duration: time.Second}
	if s.Mbps() != 1 {
		t.Errorf("mbps = %v", s.Mbps())
	}
}

func TestSampleMbpsMinWindow(t *testing.T) {
	s := Sample{Bytes: 1000000, Duration: 0}
	if got := s.Mbps(); got != 8000 { // 1 MB en 1 ms
		t.Fatalf("Mbps con ventana 0 = %v, quería 8000", got)
	}
	s.Duration = 100 * time.Microsecond
	if got := s.Mbps(); got != 8000 {
		t.Fatalf("Mbps con ventana 0,1 ms = %v", got)
	}
}
