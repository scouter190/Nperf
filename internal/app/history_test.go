package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"nperf/internal/client"
	"nperf/internal/config"
	"nperf/internal/history"
	"nperf/internal/probe"
	"nperf/internal/proto"
)

func newStore(t *testing.T) *history.Store {
	t.Helper()
	return &history.Store{Dir: filepath.Join(t.TempDir(), "historial"), Enabled: true}
}

func oneRecord(t *testing.T, s *history.Store) *history.Record {
	t.Helper()
	res, err := s.List(history.Filter{})
	if err != nil || len(res.Records) != 1 || res.Skipped != 0 {
		t.Fatalf("registros: %d, omitidos %d, err %v", len(res.Records), res.Skipped, err)
	}
	return res.Records[0]
}

func sameJSON(t *testing.T, name string, rec json.RawMessage, state any) {
	t.Helper()
	want, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var a, b any
	if err := json.Unmarshal(rec, &a); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	json.Unmarshal(want, &b)
	ga, _ := json.Marshal(a)
	gb, _ := json.Marshal(b)
	if string(ga) != string(gb) {
		t.Fatalf("%s: registro y State() difieren\n%s\n%s", name, ga, gb)
	}
}

func TestHistoryTestDoneMatchesState(t *testing.T) {
	e := newTestEnv(t, "k", "k", false)
	st := newStore(t)
	e.a.d.History = st
	if err := e.a.StartTest(); err != nil {
		t.Fatal(err)
	}
	got := e.waitStatus(t, TestDone)
	rec := oneRecord(t, st)
	if rec.Kind != history.KindTest || rec.Status != history.StatusDone || rec.Source == nil || rec.Dest == nil {
		t.Fatalf("registro: %+v", rec)
	}
	if rec.Source.Alias != "PC-LOCAL" || rec.Dest.Alias != "Dest" || rec.Dest.IP == "" {
		t.Fatalf("extremos: %+v %+v", rec.Source, rec.Dest)
	}
	sameJSON(t, "result", rec.Result, got.Report)
	sameJSON(t, "diagnosis", rec.Diagnosis, got.Diagnosis)
	sameJSON(t, "chart", rec.Chart, got.Chart)
	if s := e.a.State(); s.HistoryWarning != "" || !s.HistoryEnabled {
		t.Fatalf("estado: %+v", s)
	}
	// La clave no aparece en los archivos ni en State().
	b, _ := json.Marshal(e.a.State())
	files, _ := filepath.Glob(filepath.Join(st.Dir, "*"))
	for _, f := range files {
		data, _ := os.ReadFile(f)
		b = append(b, data...)
	}
	if strings.Contains(string(b), `"k"`) || strings.Contains(string(b), "\"key\"") {
		t.Fatalf("la clave aparece en datos: %s", b)
	}
}

func TestHistoryTestErrorSaved(t *testing.T) {
	e := newTestEnv(t, "k", "otra", false)
	st := newStore(t)
	e.a.d.History = st
	if err := e.a.StartTest(); err != nil {
		t.Fatal(err)
	}
	got := e.waitStatus(t, TestError)
	rec := oneRecord(t, st)
	if rec.Status != history.StatusError || rec.Error != got.Error || rec.Result != nil {
		t.Fatalf("registro: %+v (estado %q)", rec, got.Error)
	}
}

func TestHistoryTestCancelledSaved(t *testing.T) {
	e := newTestEnv(t, "k", "k", true)
	st := newStore(t)
	e.a.d.History = st
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	occ := make(chan struct{})
	go func() {
		defer close(occ)
		client.Run(ctx, e.addr, client.Options{Duration: time.Second, Proto: proto.ProtoTCP, Key: "k", Streams: 1})
	}()
	select {
	case <-e.entered:
	case <-time.After(15 * time.Second):
		t.Fatal("el ocupante no obtuvo turno")
	}
	if err := e.a.StartTest(); err != nil {
		t.Fatal(err)
	}
	e.a.CancelTest()
	got := e.waitStatus(t, TestCancelled)
	rec := oneRecord(t, st)
	if rec.Status != history.StatusCancelled {
		t.Fatalf("registro: %+v", rec)
	}
	if got.Chart != nil {
		sameJSON(t, "chart", rec.Chart, got.Chart)
	}
	stop()
	close(e.free)
	<-occ
}

func TestHistoryDiskFailureWarnsAndKeepsResult(t *testing.T) {
	e := newTestEnv(t, "k", "k", false)
	blocker := filepath.Join(t.TempDir(), "archivo")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	e.a.d.History = &history.Store{Dir: filepath.Join(blocker, "historial"), Enabled: true}
	if err := e.a.StartTest(); err != nil {
		t.Fatal(err)
	}
	got := e.waitStatus(t, TestDone)
	if got.Report == nil {
		t.Fatalf("resultado perdido: %+v", got)
	}
	if w := e.a.State().HistoryWarning; w != HistoryWarningText {
		t.Fatalf("aviso: %q", w)
	}
}

func TestHistoryDisabledWritesNothingAndPersists(t *testing.T) {
	e := newTestEnv(t, "k", "k", false)
	st := newStore(t)
	e.a.d.History = st
	if err := e.a.SetHistoryEnabled(false); err != nil {
		t.Fatal(err)
	}
	c, err := config.LoadFull(e.a.d.ConfigDir)
	if err != nil || c.HistoryOn() || c.Key != "k" {
		t.Fatalf("config: %+v %v", c, err)
	}
	if err := e.a.StartTest(); err != nil {
		t.Fatal(err)
	}
	e.waitStatus(t, TestDone)
	if _, err := os.Stat(st.Dir); err == nil {
		if es, _ := os.ReadDir(st.Dir); len(es) != 0 {
			t.Fatalf("desactivado escribió %d archivos", len(es))
		}
	}
	s := e.a.State()
	if s.HistoryEnabled || s.HistoryWarning != "" {
		t.Fatalf("estado: %+v", s)
	}
	if err := e.a.SetHistoryEnabled(true); err != nil {
		t.Fatal(err)
	}
	if !e.a.State().HistoryEnabled {
		t.Fatal("no se reactivó")
	}
}

func TestHistoryProbeSavedAndAPI(t *testing.T) {
	env := probe.Env{Echoer: okEcho(false), Dialer: refuseDialer{}}
	a, _ := newProbeApp(t, env, direct(), ethNet())
	st := newStore(t)
	a.d.History = st
	if err := a.StartProbe(ProbePing, hostRouter); err != nil {
		t.Fatal(err)
	}
	p := waitProbe(t, a, ProbeDone)
	rec := oneRecord(t, st)
	if rec.Kind != history.KindPing || rec.Status != history.StatusDone || rec.Dest.IP != "192.168.1.1" || rec.Dest.Alias != "Router" {
		t.Fatalf("registro: %+v dest %+v", rec, rec.Dest)
	}
	sameJSON(t, "result", rec.Result, json.RawMessage(p.Result))
	if p.Combined != nil {
		sameJSON(t, "diagnosis", rec.Diagnosis, p.Combined)
	}
	r2, err := a.HistoryRecord(rec.ID)
	if err != nil || r2.ID != rec.ID {
		t.Fatalf("HistoryRecord: %v", err)
	}
	if l, err := a.History(history.Filter{Kind: history.KindPing}); err != nil || len(l.Records) != 1 {
		t.Fatalf("History: %v %v", l, err)
	}
	if _, err := a.HistoryRecord("../x"); err == nil {
		t.Fatal("id inválido aceptado")
	}
	if n, err := a.DeleteHistory([]string{rec.ID}); err != nil || n != 1 {
		t.Fatalf("DeleteHistory: %d %v", n, err)
	}
	if err := a.StartProbe(ProbePing, hostRouter); err != nil {
		t.Fatal(err)
	}
	waitProbe(t, a, ProbeDone)
	if n, err := a.DeleteAllHistory(); err != nil || n != 1 {
		t.Fatalf("DeleteAllHistory: %d %v", n, err)
	}
}

func TestHistoryProbeErrorSaved(t *testing.T) {
	env := probe.Env{Echoer: echoFn(func(context.Context, probe.EchoRequest) (probe.Reply, error) {
		return probe.Reply{}, context.DeadlineExceeded
	}), Dialer: refuseDialer{}}
	a, _ := newProbeApp(t, env, direct(), ethNet())
	st := newStore(t)
	a.d.History = st
	if err := a.StartProbe(ProbePing, hostRouter); err != nil {
		t.Fatal(err)
	}
	a.WaitProbe(20 * time.Second)
	s := a.State().Probe
	rec := oneRecord(t, st)
	switch s.Status {
	case ProbeError:
		if rec.Status != history.StatusError || rec.Error != s.Error {
			t.Fatalf("registro: %+v vs %q", rec, s.Error)
		}
	case ProbeDone:
		if rec.Status != history.StatusDone {
			t.Fatalf("registro: %+v", rec)
		}
	default:
		t.Fatalf("estado: %+v", s)
	}
}

func TestHistoryPruneOnStart(t *testing.T) {
	st := newStore(t)
	old := time.Now().Add(-100 * 24 * time.Hour)
	st.Clock = func() time.Time { return old }
	if _, err := st.Save(&history.Record{Kind: history.KindPing, Status: history.StatusDone}); err != nil {
		t.Fatal(err)
	}
	st.Clock = nil
	if res, _ := st.List(history.Filter{}); len(res.Records) != 1 {
		t.Fatalf("antes: %d", len(res.Records))
	}
	a, _ := newProbeApp(t, probe.Env{}, nil, nil)
	a.d.History = st
	a.initHistory(config.Defaults(), true)
	if res, _ := st.List(history.Filter{}); len(res.Records) != 0 {
		t.Fatalf("Prune no borró el registro antiguo: %d", len(res.Records))
	}
}

func TestHistoryConcurrentStateAndToggle(t *testing.T) {
	env := probe.Env{Echoer: okEcho(false), Dialer: refuseDialer{}}
	a, _ := newProbeApp(t, env, direct(), ethNet())
	a.d.History = newStore(t)
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				a.State()
				a.History(history.Filter{})
				if i == 0 {
					a.SetHistoryEnabled(true)
				}
			}
		}(i)
	}
	for i := 0; i < 3; i++ {
		if err := a.StartProbe(ProbePing, hostRouter); err != nil {
			t.Fatal(err)
		}
		waitProbe(t, a, ProbeDone)
	}
	close(stop)
	wg.Wait()
}

// cfg() lee bajo el mismo cerrojo que update: nunca ve la config vacía a mitad de un guardado.
func TestConfigReadDuringUpdateKeepsHosts(t *testing.T) {
	a, _ := newProbeApp(t, probe.Env{}, nil, nil)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			_ = a.SetHistoryEnabled(i%2 == 0)
		}
	}()
	for i := 0; i < 300; i++ {
		cfg, err := a.cfg()
		if err != nil || len(cfg.ManualHosts) != 1 {
			close(stop)
			wg.Wait()
			t.Fatalf("%v %+v", err, cfg.ManualHosts)
		}
	}
	close(stop)
	wg.Wait()
}
