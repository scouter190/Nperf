package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nperf/internal/history"
)

func exportEnv(t *testing.T) (*App, []string) {
	t.Helper()
	st := newStore(t)
	var ids []string
	for i := 0; i < 2; i++ {
		id, err := st.Save(&history.Record{Kind: history.KindTest, Status: history.StatusDone,
			Source: &history.Endpoint{Alias: "A", IP: "192.168.1.2", SSID: "Casa"},
			Dest:   &history.Endpoint{Alias: "B", IP: "192.168.1.3"},
			Result: []byte(`{"format_version":1}`)})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return &App{d: Deps{History: st}}, ids
}

func TestExportHistoryFormats(t *testing.T) {
	a, ids := exportEnv(t)
	dir := t.TempDir()
	for _, c := range []struct{ f, name string }{{ExportCSV, "a.csv"}, {ExportCSVStd, "b.csv"},
		{ExportJSON, "c.json"}, {ExportHTML, "d.html"}} {
		p := filepath.Join(dir, c.name)
		w, err := a.ExportHistory(ids, c.f, ExportOpts{Client: "Cliente"}, p)
		if err != nil || len(w) != 1 || w[0] != p {
			t.Fatalf("%s: %v %v", c.f, w, err)
		}
		b, err := os.ReadFile(p)
		if err != nil || !strings.Contains(string(b), "192.168.1.2") || !strings.Contains(string(b), "Casa") {
			t.Fatalf("%s: faltan IP o SSID: %v", c.f, err)
		}
		for _, bad := range []string{"BSSID", "bssid", "mac", "key", "clave"} {
			if c.f == ExportJSON || c.f == ExportCSV || c.f == ExportCSVStd {
				if strings.Contains(strings.ToLower(string(b)), strings.ToLower(bad)) && bad != "key" && bad != "mac" {
					t.Fatalf("%s contiene %q", c.f, bad)
				}
			}
		}
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 4 {
		t.Fatalf("archivos sobrantes: %d", len(ents))
	}
}

func TestExportHistoryIntervalsFile(t *testing.T) {
	a, ids := exportEnv(t)
	p := filepath.Join(t.TempDir(), "x.csv")
	w, err := a.ExportHistory(ids, ExportCSV, ExportOpts{Intervals: true}, p)
	if err != nil || len(w) != 2 || w[1] != IntervalsPath(p) {
		t.Fatalf("%v %v", w, err)
	}
	if _, err := os.Stat(w[1]); err != nil {
		t.Fatal(err)
	}
}

func TestExportHistoryRejects(t *testing.T) {
	a, ids := exportEnv(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "x.csv")
	if _, err := a.ExportHistory([]string{"../../etc/passwd"}, ExportCSV, ExportOpts{}, p); err == nil {
		t.Fatal("id inválido aceptado")
	}
	if _, err := a.ExportHistory(ids, ExportCSV, ExportOpts{}, "  "); err == nil {
		t.Fatal("ruta vacía aceptada")
	}
	if _, err := a.ExportHistory(ids, "xml", ExportOpts{}, p); err == nil {
		t.Fatal("formato inválido aceptado")
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		t.Fatalf("se escribió algo: %d", len(ents))
	}
}
