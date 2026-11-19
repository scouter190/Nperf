package ndt7

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var day1 = time.Date(2026, 3, 10, 23, 59, 0, 0, time.Local)

func TestMain(m *testing.M) {
	if d := os.Getenv("NPERF_QUOTA_HELPER"); d != "" {
		q := &Quota{Dir: d}
		n := 0
		for i := 0; i < 3; i++ {
			if _, err := q.Reserve(day1); err == nil {
				n++
			}
		}
		os.Stdout.WriteString(strconv.Itoa(n))
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestQuota40Then41Fails(t *testing.T) {
	q := &Quota{Dir: t.TempDir()}
	for i := 1; i <= 40; i++ {
		n, err := q.Reserve(day1)
		if err != nil || n != i {
			t.Fatalf("%d: n=%d err=%v", i, n, err)
		}
	}
	if _, err := q.Reserve(day1); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("la 41 debía fallar: %v", err)
	}
}

func TestQuotaNewDayAndPersistence(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 40; i++ {
		if _, err := (&Quota{Dir: dir}).Reserve(day1); err != nil {
			t.Fatal(err)
		}
	}
	q := &Quota{Dir: dir}
	if _, err := q.Reserve(day1); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("persistencia: %v", err)
	}
	if n, err := q.Reserve(day1.Add(2 * time.Minute)); err != nil || n != 1 {
		t.Fatalf("día nuevo: n=%d err=%v", n, err)
	}
}

func TestQuotaConcurrentGoroutines(t *testing.T) {
	q := &Quota{Dir: t.TempDir()}
	var ok, full, other atomic.Int32
	var wg sync.WaitGroup
	for g := 0; g < 20; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 3; i++ {
				switch _, err := q.Reserve(day1); {
				case err == nil:
					ok.Add(1)
				case errors.Is(err, ErrQuotaExceeded):
					full.Add(1)
				default:
					other.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 40 || full.Load() != 20 || other.Load() != 0 {
		t.Fatalf("ok=%d full=%d other=%d", ok.Load(), full.Load(), other.Load())
	}
}

func TestQuotaConcurrentProcesses(t *testing.T) {
	if testing.Short() {
		t.Skip("-short")
	}
	dir := t.TempDir()
	var wg sync.WaitGroup
	var total atomic.Int32
	for p := 0; p < 15; p++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := exec.Command(os.Args[0])
			cmd.Env = append(os.Environ(), "NPERF_QUOTA_HELPER="+dir)
			out, err := cmd.Output()
			if err != nil {
				t.Error(err)
				return
			}
			n, _ := strconv.Atoi(strings.TrimSpace(string(out)))
			total.Add(int32(n))
		}()
	}
	wg.Wait()
	if total.Load() != 40 {
		t.Fatalf("procesos reservaron %d, esperado 40", total.Load())
	}
}

func TestQuotaCorruptResetsToZero(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, quotaFile), []byte("{basura"), 0o600)
	if n, err := (&Quota{Dir: dir}).Reserve(day1); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestQuotaOrphanLockBroken(t *testing.T) {
	dir := t.TempDir()
	lp := filepath.Join(dir, lockFile)
	os.WriteFile(lp, nil, 0o600)
	old := time.Now().Add(-time.Minute)
	os.Chtimes(lp, old, old)
	if n, err := (&Quota{Dir: dir}).Reserve(day1); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestQuotaFreshLockTimesOut(t *testing.T) {
	if testing.Short() {
		t.Skip("-short")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, lockFile), nil, 0o600)
	if _, err := (&Quota{Dir: dir}).Reserve(day1); err == nil || errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("esperaba error de E/S: %v", err)
	}
}

func TestQuotaIOErrorNoMeasure(t *testing.T) {
	f := filepath.Join(t.TempDir(), "archivo")
	os.WriteFile(f, nil, 0o600)
	// Dir es un archivo: no se puede crear.
	if _, err := (&Quota{Dir: filepath.Join(f, "sub")}).Reserve(day1); err == nil {
		t.Fatal("esperaba error")
	}
}

func TestQuotaReleaseKeepsForeignLock(t *testing.T) {
	q := &Quota{Dir: t.TempDir()}
	unlock, err := q.lock()
	if err != nil {
		t.Fatal(err)
	}
	lp := filepath.Join(q.Dir, lockFile)
	os.WriteFile(lp, []byte("otro-token"), 0o600) // otro proceso rompió y recreó el bloqueo
	unlock()
	if _, err := os.Stat(lp); err != nil {
		t.Fatalf("no debía borrar el bloqueo ajeno: %v", err)
	}
	unlock2, _ := func() (func(), error) { os.Remove(lp); return q.lock() }()
	unlock2()
	if _, err := os.Stat(lp); err == nil {
		t.Fatal("el bloqueo propio debía borrarse")
	}
}
