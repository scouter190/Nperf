package history

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func put(t *testing.T, dir, id, kind, tm, host string) {
	t.Helper()
	r := sample(kind)
	r.ID, r.Time = id, tm
	r.Dest = &Endpoint{Alias: host, IP: "10.0.0.9"}
	b, err := r.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+".json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestListFiltersOrderSkipped(t *testing.T) {
	d := t.TempDir()
	put(t, d, "20260101-100000-00000001", KindTest, "2026-01-01T10:00:00Z", "pc1")
	put(t, d, "20260102-100000-00000002", KindPing, "2026-01-02T10:00:00Z", "pc2")
	put(t, d, "20260103-100000-00000003", KindTest, "2026-01-03T10:00:00Z", "PC1")
	os.WriteFile(filepath.Join(d, "20260104-100000-00000004.json"), []byte("{no"), 0o600)
	os.WriteFile(filepath.Join(d, "20260105-100000-00000005.json"), []byte(`{"format_version":"x"}`), 0o600)
	os.WriteFile(filepath.Join(d, "20260106-100000-00000006.json"), []byte(strings.Repeat(" ", MaxRecordBytes+1)), 0o600)
	os.WriteFile(filepath.Join(d, "otro.json"), []byte("x"), 0o600)
	os.WriteFile(filepath.Join(d, "notas.txt"), []byte("x"), 0o600)

	res, err := ListDir(d, Filter{})
	if err != nil || res.Skipped != 3 || len(res.Records) != 3 {
		t.Fatal(res.Skipped, len(res.Records), err)
	}
	if res.Records[0].ID != "20260103-100000-00000003" || res.Records[2].ID != "20260101-100000-00000001" {
		t.Fatal("orden incorrecto")
	}
	res, _ = ListDir(d, Filter{Host: "pc1", Kind: KindTest})
	if len(res.Records) != 2 {
		t.Fatal(len(res.Records))
	}
	res, _ = ListDir(d, Filter{Host: "pc1", Since: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), Until: time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC)})
	if len(res.Records) != 1 || res.Records[0].ID != "20260103-100000-00000003" {
		t.Fatal(res.Records)
	}
	if res, err := ListDir(filepath.Join(d, "no"), Filter{}); err != nil || len(res.Records) != 0 {
		t.Fatal(err)
	}
}

func TestGetDeleteAll(t *testing.T) {
	d := t.TempDir()
	ids := []string{"20260101-100000-00000001", "20260102-100000-00000002", "20260103-100000-00000003"}
	for _, id := range ids {
		put(t, d, id, KindTest, "2026-01-01T10:00:00Z", "pc")
	}
	ajeno := filepath.Join(d, "ajeno.txt")
	os.WriteFile(ajeno, []byte("x"), 0o600)
	if r, err := GetDir(d, ids[0]); err != nil || r.ID != ids[0] {
		t.Fatal(err)
	}
	for _, bad := range []string{"../x", "", "a/b", `a\b`} {
		if _, err := GetDir(d, bad); err == nil {
			t.Fatal("get aceptó", bad)
		}
		if _, err := DeleteDir(d, []string{ids[0], bad}); err == nil {
			t.Fatal("delete aceptó", bad)
		}
	}
	if _, err := GetDir(d, ids[0]); err != nil {
		t.Fatal("borró pese a id inválido")
	}
	if n, err := DeleteDir(d, []string{ids[0]}); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if n, err := DeleteDir(d, []string{ids[1], "20260109-100000-000000ff"}); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if n, err := DeleteAllDir(d); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if _, err := os.Stat(ajeno); err != nil {
		t.Fatal("borró archivo ajeno")
	}
}

func TestSymlinkNotFollowedAndStoreMethods(t *testing.T) {
	d := t.TempDir()
	out := t.TempDir()
	id := "20260101-100000-00000001"
	put(t, out, id, KindTest, "2026-01-01T10:00:00Z", "pc")
	if err := os.Symlink(filepath.Join(out, id+".json"), filepath.Join(d, id+".json")); err != nil {
		t.Skip("sin symlinks:", err)
	}
	s := &Store{Dir: d}
	if _, err := s.Get(id); err == nil {
		t.Fatal("siguió el symlink")
	}
	if res, _ := s.List(Filter{}); len(res.Records) != 0 || res.Skipped != 1 {
		t.Fatal(res)
	}
	if n, _ := s.Delete([]string{id}); n != 0 {
		t.Fatal("borró symlink")
	}
	if n, _ := s.DeleteAll(); n != 0 {
		t.Fatal("borró symlink")
	}
	if _, err := os.Stat(filepath.Join(out, id+".json")); err != nil {
		t.Fatal(err)
	}
}

func TestSkippedWrongTypes(t *testing.T) {
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, "20260101-100000-00000001.json"), []byte(`{"format_version":1,"id":"20260101-100000-00000001","time":5,"kind":"test","status":"done"}`), 0o600)
	s := &Store{Dir: d}
	if res, _ := s.List(Filter{}); res.Skipped != 1 || len(res.Records) != 0 {
		t.Fatal(res)
	}
}
