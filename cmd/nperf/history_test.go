package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nperf/internal/history"
)

func histSetup(t *testing.T) (dir string, ids []string) {
	t.Helper()
	dir = t.TempDir()
	old := historyDir
	historyDir = func() string { return dir }
	t.Cleanup(func() { historyDir = old })
	st := &history.Store{Dir: dir, Enabled: true, Clock: func() time.Time { return time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC) }}
	for i, r := range []*history.Record{
		{Kind: history.KindPing, Status: history.StatusDone, Time: "2026-01-01T10:00:00Z",
			Dest: &history.Endpoint{IP: "10.0.0.2", Alias: "Taller"}, Result: json.RawMessage(`{"sent":4}`)},
		{Kind: history.KindTest, Status: history.StatusDone, Time: "2026-03-01T10:00:00Z",
			Source: &history.Endpoint{Alias: "Yo"}, Dest: &history.Endpoint{IP: "10.0.0.3"}},
	} {
		r.ID = "2026010" + string(rune('1'+i)) + "-100000-0000000" + string(rune('a'+i))
		id, err := st.Save(r)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return dir, ids
}

func TestHistoryDispatch(t *testing.T) {
	// run no conoce aún "history" (T23): se prueba runHistory directamente.
	var o, e bytes.Buffer
	so, se := stdout, stderr
	stdout, stderr = &o, &e
	defer func() { stdout, stderr = so, se }()
	if c := runHistory(nil); c != exitUsage {
		t.Fatalf("code %d", c)
	}
	if c := runHistory([]string{"zzz"}); c != exitUsage {
		t.Fatalf("code %d", c)
	}
}

func hist(args ...string) (int, string, string) {
	var o, e bytes.Buffer
	so, se := stdout, stderr
	stdout, stderr = &o, &e
	defer func() { stdout, stderr = so, se }()
	c := runHistory(args)
	return c, o.String(), e.String()
}

func TestHistoryListTextAndFilters(t *testing.T) {
	_, ids := histSetup(t)
	c, out, _ := hist("list")
	if c != 0 || !strings.Contains(out, ids[0]) || !strings.Contains(out, ids[1]) {
		t.Fatalf("%d %q", c, out)
	}
	if strings.Index(out, ids[1]) > strings.Index(out, ids[0]) {
		t.Fatal("orden: más reciente primero")
	}
	_, out, _ = hist("list", "-kind", "ping")
	if !strings.Contains(out, ids[0]) || strings.Contains(out, ids[1]) {
		t.Fatalf("kind: %q", out)
	}
	_, out, _ = hist("list", "-host", "taller")
	if !strings.Contains(out, ids[0]) || strings.Contains(out, ids[1]) {
		t.Fatalf("host: %q", out)
	}
	_, out, _ = hist("list", "-since", "2026-02-01")
	if strings.Contains(out, ids[0]) || !strings.Contains(out, ids[1]) {
		t.Fatalf("since: %q", out)
	}
}

func TestHistoryListJSON(t *testing.T) {
	_, ids := histSetup(t)
	c, out, errs := hist("list", "-json")
	if c != 0 || errs != "" {
		t.Fatalf("%d %q", c, errs)
	}
	var v struct {
		FormatVersion int        `json:"format_version"`
		Command       string     `json:"command"`
		Records       []listItem `json:"records"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatal(err, out)
	}
	if v.FormatVersion != 1 || v.Command != "history list" || len(v.Records) != 2 || v.Records[0].ID != ids[1] {
		t.Fatalf("%+v", v)
	}
}

func TestHistoryListEmptyAndUsage(t *testing.T) {
	dir := t.TempDir()
	old := historyDir
	historyDir = func() string { return filepath.Join(dir, "no") }
	defer func() { historyDir = old }()
	if c, out, _ := hist("list"); c != 0 || !strings.Contains(out, "No hay registros") {
		t.Fatalf("%d %q", c, out)
	}
	for _, a := range [][]string{{"list", "-kind", "x"}, {"list", "-since", "ayer"}, {"list", "-zzz"}, {"list", "extra"}} {
		if c, _, _ := hist(a...); c != exitUsage {
			t.Errorf("%v: code %d", a, c)
		}
	}
	c, out, _ := hist("list", "-json", "-kind", "x")
	if c != exitUsage || !strings.Contains(out, `"error"`) {
		t.Fatalf("%d %q", c, out)
	}
}

func TestHistoryShow(t *testing.T) {
	_, ids := histSetup(t)
	c, out, _ := hist("show", ids[0])
	if c != 0 || !strings.Contains(out, "ping") || !strings.Contains(out, "Taller") {
		t.Fatalf("%d %q", c, out)
	}
	c, out, _ = hist("show", ids[0], "-json")
	var v struct {
		FormatVersion int            `json:"format_version"`
		Command       string         `json:"command"`
		Record        history.Record `json:"record"`
	}
	if err := json.Unmarshal([]byte(out), &v); c != 0 || err != nil || v.Command != "history show" || v.Record.ID != ids[0] || v.FormatVersion != 1 {
		t.Fatalf("%d %v %q", c, err, out)
	}
	if c, _, _ := hist("show", "20260101-100000-0000dead"); c != exitError {
		t.Fatalf("inexistente: %d", c)
	}
	c, out, _ = hist("show", "20260101-100000-0000dead", "-json")
	if c != exitError || !strings.Contains(out, `"error"`) {
		t.Fatalf("%d %q", c, out)
	}
	for _, a := range [][]string{{"show"}, {"show", "../x"}} {
		if c, _, _ := hist(a...); c != exitUsage {
			t.Errorf("%v: %d", a, c)
		}
	}
}

func TestHistoryExport(t *testing.T) {
	_, ids := histSetup(t)
	historyNow = func() time.Time { return time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC) }
	defer func() { historyNow = time.Now }()
	o := filepath.Join(t.TempDir(), "out")

	c, _, errs := hist("export", ids[0], "-format", "csv", "-o", o+".csv")
	b, _ := os.ReadFile(o + ".csv")
	if c != 0 || !strings.Contains(string(b), ids[0]) || strings.Contains(string(b), ids[1]) || !strings.Contains(string(b), ",") {
		t.Fatalf("%d %q %q", c, errs, b)
	}
	if c, _, _ := hist("export", "-all", "-format", "csv", "-csv", "excel-es", "-o", o+"2.csv"); c != 0 {
		t.Fatal(c)
	}
	b, _ = os.ReadFile(o + "2.csv")
	if !strings.Contains(string(b), ";") || !strings.Contains(string(b), ids[1]) {
		t.Fatalf("%q", b)
	}
	if c, _, _ := hist("export", "-format", "html", "-o", o+".html", ids[0], ids[1]); c != 0 {
		t.Fatal(c)
	}
	if b, _ = os.ReadFile(o + ".html"); !strings.Contains(string(b), "<html") {
		t.Fatalf("%q", b)
	}
	if c, _, _ := hist("export", "-all", "-format", "json", "-o", o+".json"); c != 0 {
		t.Fatal(c)
	}
	var v struct {
		Command string           `json:"command"`
		Records []history.Record `json:"records"`
	}
	b, _ = os.ReadFile(o + ".json")
	if err := json.Unmarshal(b, &v); err != nil || v.Command != "history export" || len(v.Records) != 2 {
		t.Fatalf("%v %q", err, b)
	}
}

func TestHistoryExportErrors(t *testing.T) {
	_, ids := histSetup(t)
	o := filepath.Join(t.TempDir(), "x")
	usage := [][]string{
		{"export", ids[0], "-o", o},
		{"export", ids[0], "-format", "pdf", "-o", o},
		{"export", ids[0], "-format", "csv"},
		{"export", "-format", "csv", "-o", o},
		{"export", "-all", ids[0], "-format", "csv", "-o", o},
		{"export", ids[0], "-format", "csv", "-csv", "x", "-o", o},
		{"export", "../x", "-format", "csv", "-o", o},
	}
	for _, a := range usage {
		if c, _, _ := hist(a...); c != exitUsage {
			t.Errorf("%v: %d", a, c)
		}
	}
	if c, _, _ := hist("export", "20260101-100000-0000dead", "-format", "csv", "-o", o); c != exitError {
		t.Fatalf("inexistente: %d", c)
	}
	if _, err := os.Stat(o); err == nil {
		t.Fatal("no debe crear archivo")
	}
	if c, _, _ := hist("export", ids[0], "-format", "csv", "-o", filepath.Join(o, "no", "x")); c != exitError {
		t.Fatalf("ruta inválida: %d", c)
	}
}
