package client

import (
	"testing"
	"time"
)

func lateFixture(sentAgo time.Duration) (*sess, *Dir, *[]Interval) {
	var got []Interval
	lv := &liveRun{phase: "tcp_fwd", onInterval: func(_ string, iv Interval) { got = append(got, iv) },
		ss: &sess{}, initAvg: time.Millisecond}
	ss := lv.ss
	d := &Dir{Intervals: []Interval{{Index: 0, HasRTT: true, RTT: time.Millisecond}, {Index: 1}},
		LoadedRTT: NewLoadedRTT([]time.Duration{time.Millisecond}, 2)}
	ss.late = &lateProbe{lv: lv, seq: 1, sentAt: hrNow().Add(-sentAgo), iv: Interval{Index: 1},
		answered: []time.Duration{time.Millisecond}, sent: 2, dst: d}
	return ss, d, &got
}

func TestLatePongAssignedToLastSample(t *testing.T) {
	ss, d, got := lateFixture(10 * time.Millisecond)
	ss.latePong(0) // otro seq: se descarta
	if ss.late == nil || len(*got) != 0 {
		t.Fatal("un pong ajeno no debe resolver")
	}
	ss.latePong(1)
	if ss.late != nil || len(*got) != 1 || !(*got)[0].HasRTT {
		t.Fatalf("got %+v late=%v", *got, ss.late)
	}
	if !d.Intervals[1].HasRTT || d.LoadedRTT.Samples != 2 {
		t.Fatalf("dir sin parchear: %+v %+v", d.Intervals, d.LoadedRTT)
	}
}

func TestLatePongTooLateOrGivenUp(t *testing.T) {
	ss, d, got := lateFixture(1500 * time.Millisecond)
	ss.latePong(1)
	if len(*got) != 1 || (*got)[0].HasRTT || d.Intervals[1].HasRTT || d.LoadedRTT.Samples != 1 {
		t.Fatalf("pong tardío: %+v", *got)
	}
	ss, _, got = lateFixture(10 * time.Millisecond)
	ss.giveUpLate()
	ss.giveUpLate()
	if len(*got) != 1 || (*got)[0].HasRTT {
		t.Fatalf("giveUp: %+v", *got)
	}
}
