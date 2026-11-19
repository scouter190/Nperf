package export

import (
	"encoding/csv"
	"flag"
	"os"
	"strings"
	"testing"

	"nperf/internal/history"
)

var update = flag.Bool("update", false, "regenera los golden")

const testResult = `{"rtt":{"status":"ok","min_ms":1,"avg_ms":2.5,"max_ms":4},
"tcp":{"fwd":{"status":"ok","throughput_mbps":941.123456,"bytes":1000,"duration_s":10,"retransmissions":{"status":"available","count":3,"estimated":true},
"intervals":[{"t_s":0,"duration_s":1,"bytes":100,"throughput_mbps":-1.5,"rtt_ms":null,"datagrams_received":null,"datagrams_lost":null,"loss_pct":null,"jitter_ms":null}]},
"rev":{"status":"unavailable","throughput_mbps":null,"bytes":null,"duration_s":null,"retransmissions":null,"udp":null,"intervals":null}},
"udp":{"fwd":{"status":"ok","throughput_mbps":90.5,"bytes":5,"duration_s":10,"udp":{"loss_pct":0.25,"jitter_ms":0.5},
"intervals":[{"t_s":1,"duration_s":1,"bytes":5,"throughput_mbps":90,"rtt_ms":1.5,"datagrams_received":10,"datagrams_lost":1,"loss_pct":9.1,"jitter_ms":0.2}]},
"rev":{"status":"unavailable"}}}`

func hostile() []*history.Record {
	return []*history.Record{
		{ID: "20260101-000000-aaaaaaaa", Time: "2026-01-01T00:00:00Z", Kind: "test", Status: "done",
			Source: &history.Endpoint{Alias: "=HYPERLINK(\"http://x\")", IP: "10.0.0.1"},
			Dest:   &history.Endpoint{Alias: "a;b,c \"q\"\nlinea", IP: "10.0.0.2", SSID: "@SUM(1)"},
			Result: []byte(testResult)},
		{ID: "20260101-000001-bbbbbbbb", Time: "2026-01-01T00:00:01Z", Kind: "ping", Status: "error",
			Error: "+cmd|' /C calc'!A0", Dest: &history.Endpoint{Alias: "-2+3", IP: "\tx"},
			Result: []byte(`{"status":"ok","loss_pct":0,"min_ms":1,"avg_ms":2,"max_ms":3,"jitter_ms":0.5}`)},
		{ID: "20260101-000002-cccccccc", Time: "2026-01-01T00:00:02Z", Kind: "speed", Status: "done",
			Result: []byte(`{"download":{"mbps":50.5,"bytes":9,"duration_s":10,"min_rtt_ms":12},"upload":null}`)},
		{ID: "20260101-000003-dddddddd", Time: "2026-01-01T00:00:03Z", Kind: "mtu", Status: "cancelled"},
	}
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	p := "testdata/" + name
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(got) {
		t.Fatalf("%s difiere:\n%s", name, got)
	}
}

func TestCSVGolden(t *testing.T) {
	for _, c := range []struct {
		name string
		f    CSVFormat
	}{{"std", CSVStd}, {"excel-es", CSVExcelES}} {
		s, err := SummaryCSV(hostile(), c.f)
		if err != nil {
			t.Fatal(err)
		}
		golden(t, "summary-"+c.name+".csv", s)
		i, err := IntervalsCSV(hostile(), c.f)
		if err != nil {
			t.Fatal(err)
		}
		golden(t, "intervals-"+c.name+".csv", i)
	}
}

func parse(t *testing.T, b []byte, comma rune) [][]string {
	t.Helper()
	if !strings.HasPrefix(string(b), "\xef\xbb\xbf") {
		t.Fatal("sin BOM")
	}
	r := csv.NewReader(strings.NewReader(string(b[3:])))
	r.Comma = comma
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestCSVSecurityAndShape(t *testing.T) {
	for _, c := range []struct {
		f     CSVFormat
		comma rune
		dec   string
		neg   string
	}{{CSVStd, ',', "941.123", "-1.5"}, {CSVExcelES, ';', "941,123", "-1,5"}} {
		b, _ := SummaryCSV(hostile(), c.f)
		rows := parse(t, b, c.comma)
		if len(rows[0]) != len(SummaryHeader) {
			t.Fatal("cabecera")
		}
		for _, r := range rows[1:] {
			if len(r) != len(SummaryHeader) {
				t.Fatalf("fila con %d columnas", len(r))
			}
			for _, cell := range r {
				if cell != "" && strings.ContainsAny(cell[:1], "=+@\t\r") {
					t.Fatalf("celda sin neutralizar: %q", cell)
				}
			}
		}
		if len(rows) != 1+4+1+2+1 { // tcp 2 + udp 2, ping, speed 2, mtu
			t.Fatalf("filas: %d", len(rows))
		}
		r1 := rows[1]
		if r1[5] != "'=HYPERLINK(\"http://x\")" || r1[8] != "a;b,c \"q\"\nlinea" || r1[10] != "'@SUM(1)" {
			t.Fatalf("texto hostil: %q", r1)
		}
		if r1[14] != c.dec {
			t.Fatalf("decimal %q", r1[14])
		}
		if rows[2][13] != "unavailable" || rows[2][14] != "" {
			t.Fatalf("no medido: %q", rows[2])
		}
		if rows[5][8] != "'-2+3" || rows[5][4] != "'+cmd|' /C calc'!A0" {
			t.Fatalf("neutralización: %q", rows[5])
		}
		ib, _ := IntervalsCSV(hostile(), c.f)
		ir := parse(t, ib, c.comma)
		if ir[1][7] != c.neg {
			t.Fatalf("negativo alterado: %q", ir[1])
		}
	}
}
