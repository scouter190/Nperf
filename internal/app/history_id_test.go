package app

import (
	"os"
	"path/filepath"
	"testing"

	"nperf/internal/history"
	"nperf/internal/probe"
)

func TestHistoryIDTest(t *testing.T) {
	e := newTestEnv(t, "k", "k", false)
	st := newStore(t)
	e.a.d.History = st
	if err := e.a.StartTest(); err != nil {
		t.Fatal(err)
	}
	got := e.waitStatus(t, TestDone)
	rec := oneRecord(t, st)
	if got.HistoryID == "" || got.HistoryID != rec.ID {
		t.Fatalf("history_id %q, registro %q", got.HistoryID, rec.ID)
	}
	if _, err := e.a.DeleteHistory([]string{rec.ID}); err != nil {
		t.Fatal(err)
	}
	if id := e.a.State().Test.HistoryID; id != "" {
		t.Fatalf("no se vació: %q", id)
	}
}

func TestHistoryIDTestEmptyWhenDisabledOrFailing(t *testing.T) {
	e := newTestEnv(t, "k", "k", false)
	st := newStore(t)
	st.Enabled = false
	e.a.d.History = st
	if err := e.a.StartTest(); err != nil {
		t.Fatal(err)
	}
	if got := e.waitStatus(t, TestDone); got.HistoryID != "" {
		t.Fatalf("desactivado: %q", got.HistoryID)
	}
	e2 := newTestEnv(t, "k", "k", false)
	blocker := filepath.Join(t.TempDir(), "archivo")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	e2.a.d.History = &history.Store{Dir: filepath.Join(blocker, "h"), Enabled: true}
	if err := e2.a.StartTest(); err != nil {
		t.Fatal(err)
	}
	if got := e2.waitStatus(t, TestDone); got.HistoryID != "" {
		t.Fatalf("fallo al guardar: %q", got.HistoryID)
	}
}

func TestHistoryIDProbeAndInternet(t *testing.T) {
	env := probe.Env{Echoer: okEcho(false), Dialer: refuseDialer{}}
	a, _ := newProbeApp(t, env, direct(), ethNet())
	st := newStore(t)
	a.d.History = st
	if err := a.StartProbe(ProbePing, hostRouter); err != nil {
		t.Fatal(err)
	}
	p := waitProbe(t, a, ProbeDone)
	rec := oneRecord(t, st)
	if p.HistoryID == "" || p.HistoryID != rec.ID {
		t.Fatalf("diagnóstico: %q vs %q", p.HistoryID, rec.ID)
	}
	if n, err := a.DeleteAllHistory(); err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	if id := a.State().Probe.HistoryID; id != "" {
		t.Fatalf("no se vació: %q", id)
	}

	a.d.Ping = probe.PingOptions{Count: 1, Interval: probe.MinInterval, Timeout: probe.MinTimeout}
	if err := a.SetInternetTargets([]string{"9.9.9.9"}); err != nil {
		t.Fatal(err)
	}
	if err := a.StartInternet(); err != nil {
		t.Fatal(err)
	}
	p = waitProbe(t, a, ProbeDone)
	rec = oneRecord(t, st)
	if p.HistoryID == "" || p.HistoryID != rec.ID {
		t.Fatalf("internet: %q vs %q", p.HistoryID, rec.ID)
	}
	if err := a.SetHistoryEnabled(false); err != nil {
		t.Fatal(err)
	}
	if err := a.StartInternet(); err != nil {
		t.Fatal(err)
	}
	if p = waitProbe(t, a, ProbeDone); p.HistoryID != "" {
		t.Fatalf("desactivado: %q", p.HistoryID)
	}
}
