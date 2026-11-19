package history

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var t0 = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

func rec(id string) *Record {
	return &Record{ID: id, Kind: KindPing, Status: StatusDone}
}

func names(t *testing.T, dir string) []string {
	t.Helper()
	des, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var n []string
	for _, d := range des {
		n = append(n, d.Name())
	}
	return n
}

func TestDisabledDoesNotWrite(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "h")
	s, err := Open(dir, func() time.Time { return t0 }, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(rec("")); !errors.Is(err, ErrDisabled) {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("no debía crear el directorio")
	}
}

func TestSaveAtomicUniqueAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir, func() time.Time { return t0 }, true)
	a, err := s.Save(rec(""))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := s.Save(rec(""))
	if a == b || !ValidID(a) {
		t.Fatalf("ids %q %q", a, b)
	}
	data, err := os.ReadFile(filepath.Join(dir, a+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if r, err := Decode(data); err != nil || r.ID != a {
		t.Fatalf("decode: %v", err)
	}
	if _, err := s.Save(rec(a)); err == nil {
		t.Fatal("id repetido debía fallar")
	}
	for _, n := range names(t, dir) {
		if !strings.HasSuffix(n, ".json") {
			t.Fatalf("residuo %s", n)
		}
	}
}

func TestRenameFailureLeavesDirClean(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir, func() time.Time { return t0 }, true)
	s.linkFn = func(string, string) error { return errors.New("boom") }
	s.renameFn = func(string, string) error { return errors.New("boom") }
	if _, err := s.Save(rec("")); err == nil {
		t.Fatal("esperaba error")
	}
	if n := names(t, dir); len(n) != 0 {
		t.Fatalf("residuos %v", n)
	}
}

func idAt(tm time.Time, n int) string {
	return fmt.Sprintf("%s-%08x", tm.UTC().Format("20060102-150405"), n)
}

func TestRetentionByAge(t *testing.T) {
	dir := t.TempDir()
	now := t0
	s := &Store{Dir: dir, Clock: func() time.Time { return now }, Enabled: true}
	old := idAt(now.Add(-DefaultMaxAge-time.Second), 1)
	edge := idAt(now.Add(-DefaultMaxAge), 2)
	fresh := idAt(now.Add(-time.Hour), 3)
	for _, id := range []string{old, edge, fresh} {
		os.WriteFile(filepath.Join(dir, id+".json"), []byte("{}"), 0o600)
	}
	os.WriteFile(filepath.Join(dir, "ajeno.txt"), []byte("x"), 0o600)
	s.Prune()
	got := strings.Join(names(t, dir), ",")
	want := edge + ".json," + fresh + ".json,ajeno.txt"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestRetentionBySizeExact(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Dir: dir, Clock: func() time.Time { return t0 }, Enabled: true, MaxBytes: 300}
	var ids []string
	for i := 0; i < 4; i++ {
		id := idAt(t0.Add(-time.Duration(4-i)*time.Hour), i)
		ids = append(ids, id)
		os.WriteFile(filepath.Join(dir, id+".json"), bytes.Repeat([]byte("a"), 100), 0o600)
	}
	s.Prune() // 400 > 300: borra el más antiguo; 300 queda (no > límite)
	got := names(t, dir)
	if len(got) != 3 || got[0] != ids[1]+".json" {
		t.Fatalf("got %v", got)
	}
	s.Prune()
	if len(names(t, dir)) != 3 {
		t.Fatal("exactamente en el límite no debe borrar")
	}
}

func TestConcurrentWriters(t *testing.T) {
	dir := t.TempDir()
	clock := func() time.Time { return t0 }
	s1, _ := Open(dir, clock, true)
	s2, _ := Open(dir, clock, true)
	var wg sync.WaitGroup
	for _, s := range []*Store{s1, s2} {
		wg.Add(1)
		go func(s *Store) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				if _, err := s.Save(rec("")); err != nil {
					t.Error(err)
				}
			}
		}(s)
	}
	wg.Wait()
	if n := len(names(t, dir)); n != 100 {
		t.Fatalf("archivos = %d", n)
	}
}

func TestSaveDoesNotOverwriteAndRetries(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir, func() time.Time { return t0 }, true)
	first := true
	s.rand = nil
	s.linkFn = func(o, n string) error {
		if first { // otro proceso creó el destino entre medias
			first = false
			os.WriteFile(n, []byte("ajeno"), 0o600)
		}
		return os.Link(o, n)
	}
	id, err := s.Save(rec(""))
	if err != nil {
		t.Fatal(err)
	}
	n := names(t, dir)
	if len(n) != 2 {
		t.Fatalf("archivos %v", n)
	}
	b, _ := os.ReadFile(filepath.Join(dir, id+".json"))
	if string(b) == "ajeno" {
		t.Fatal("pisó el archivo")
	}
	// id explícito existente: error y el original intacto.
	s.linkFn = nil
	if _, err := s.Save(rec(id)); err == nil {
		t.Fatal("esperaba error")
	}
}

func TestPruneRemovesOrphanTemps(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Dir: dir, Clock: func() time.Time { return t0 }, Enabled: true}
	oldT, newT := filepath.Join(dir, ".tmp-old"), filepath.Join(dir, ".tmp-new")
	os.WriteFile(oldT, []byte("x"), 0o600)
	os.WriteFile(newT, []byte("x"), 0o600)
	os.Chtimes(oldT, t0.Add(-2*time.Hour), t0.Add(-2*time.Hour))
	os.Chtimes(newT, t0.Add(-time.Minute), t0.Add(-time.Minute))
	s.Prune()
	if got := names(t, dir); len(got) != 1 || got[0] != ".tmp-new" {
		t.Fatalf("got %v", got)
	}
}

func TestSaveFallsBackWhenLinkUnsupported(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir, func() time.Time { return t0 }, true)
	s.linkFn = func(string, string) error { return errors.New("no soportado") }
	id, err := s.Save(rec(""))
	if err != nil {
		t.Fatal(err)
	}
	if n := names(t, dir); len(n) != 1 || n[0] != id+".json" {
		t.Fatalf("archivos %v", n)
	}
	if _, err := s.Save(rec(id)); err == nil {
		t.Fatal("id existente no debe pisarse")
	}
}
