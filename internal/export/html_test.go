package export

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"nperf/internal/history"
)

var reportNow = time.Date(2026, 3, 5, 10, 30, 0, 0, time.UTC)

const (
	secretKey   = "CLAVE-SECRETA-123"
	secretMAC   = "aa:bb:cc:dd:ee:ff"
	secretBSSID = "11:22:33:44:55:66"
)

const reportChart = `{"phases":[{"phase":"tcp_fwd","label":"<script>alert(1)</script> \"A\" → B","nominal_mbps":1000,
"rtt_initial":{"min_ms":1,"avg_ms":2,"max_ms":3},
"samples":[[0,1,900,2.5,null,null],[1,1,940,null,null,null],[2,1,950,3,null,null]],"available":true},
{"phase":"udp_fwd","label":"UDP","samples":[],"available":false,"note":"<img src=x onerror=alert(1)>"}]}`

const reportDiag = `{"fwd":{"color":"green","percent":94,"text":"Va bien <b>negrita</b>","partial":false},
"rev":{"color":"red","percent":20,"text":"Muy lento","partial":true},"local":{"kind":"wifi"}}`

func reportRecords(mlab bool) []*history.Record {
	recs := []*history.Record{
		{ID: "20260101-000000-aaaaaaaa", Time: "2026-01-01T09:00:00+01:00", Kind: "test", Status: "done",
			Version:   "1.0",
			Source:    &history.Endpoint{Alias: "<script>alert('a')</script>", IP: "10.0.0.1", Link: "wifi", SSID: "<script>ssid</script>", NominalMbps: 866},
			Dest:      &history.Endpoint{Alias: "\"><img src=x onerror=alert(2)>", IP: "10.0.0.2", Link: "eth", NominalMbps: 1000},
			Params:    []byte(`{"key":"` + secretKey + `","mac":"` + secretMAC + `"}`),
			Result:    []byte(testResult[:0] + `{"key":"` + secretKey + `","bssid":"` + secretBSSID + `",` + testResult[1:]),
			Diagnosis: []byte(reportDiag), Chart: []byte(reportChart),
			Notes: []string{"nota <script>x</script>"}},
		{ID: "20260101-000001-bbbbbbbb", Time: "2026-01-01T09:05:00Z", Kind: "ping", Status: "error",
			Error: "fallo <b>x</b>", Dest: &history.Endpoint{Alias: "router", IP: "192.168.1.1"},
			Result: []byte(`{"loss_pct":1.5,"min_ms":1,"avg_ms":2,"max_ms":3,"jitter_ms":0.5}`)},
		{ID: "20260101-000002-cccccccc", Time: "2026-01-01T09:06:00Z", Kind: "mtu", Status: "cancelled"},
	}
	if mlab {
		recs = append(recs, &history.Record{ID: "20260101-000003-dddddddd", Time: "2026-01-01T09:07:00Z",
			Kind: "speed", Status: "done",
			Result: []byte(`{"download":{"mbps":50.5,"bytes":9,"duration_s":10,"min_rtt_ms":12},"upload":{"mbps":10,"incomplete":true}}`)})
	}
	return recs
}

var reportOpts = ReportOptions{Client: "Cliente <script>c</script>", Technician: "Téc \"x\"",
	Notes: "Línea 1\n<script>alert(3)</script>\nLínea 3"}

func TestReportGolden(t *testing.T) {
	for name, mlab := range map[string]bool{"report-sin-mlab.html": false, "report-mlab.html": true} {
		b, err := ReportHTML(reportRecords(mlab), reportOpts, reportNow)
		if err != nil {
			t.Fatal(err)
		}
		golden(t, name, b)
	}
}

var extURL = regexp.MustCompile(`(?i)(src|href)\s*=\s*["']?\s*(https?:|//|data:|javascript:)|url\(|@import`)

func TestReportSafety(t *testing.T) {
	for _, mlab := range []bool{false, true} {
		b, err := ReportHTML(reportRecords(mlab), reportOpts, reportNow)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		low := strings.ToLower(s)
		for _, bad := range []string{"<script", "<img", "<b>", "<iframe", "<link", "<object", "<embed"} {
			if strings.Contains(low, bad) {
				t.Errorf("mlab=%v: contiene %q", mlab, bad)
			}
		}
		for _, bad := range []string{secretKey, secretMAC, secretBSSID} {
			if strings.Contains(s, bad) {
				t.Errorf("mlab=%v: contiene dato prohibido %q", mlab, bad)
			}
		}
		if !strings.Contains(s, "script-src") && !strings.Contains(s, "default-src 'none'") {
			t.Error("falta CSP que prohíba scripts")
		}
		if !strings.Contains(s, `http-equiv="Content-Security-Policy"`) {
			t.Error("falta la meta CSP")
		}
		if !strings.Contains(s, "&lt;script&gt;") {
			t.Error("el alias hostil debe aparecer escapado")
		}
		// Los únicos http(s) permitidos: el enlace de atribución cuando hay M-Lab.
		for _, m := range regexp.MustCompile(`https?://[^\s"'<>]+`).FindAllString(s, -1) {
			if m == "http://www.w3.org/2000/svg" { // espacio de nombres XML, no un recurso
				continue
			}
			if !mlab || m != MLabURL {
				t.Errorf("mlab=%v: URL no permitida %q", mlab, m)
			}
		}
		for _, m := range extURL.FindAllString(strings.ReplaceAll(s, `href="`+MLabURL+`"`, ""), -1) {
			t.Errorf("mlab=%v: recurso externo %q", mlab, m)
		}
		if mlab != strings.Contains(s, MLabNotice) || mlab != strings.Contains(s, "M-Lab NDT7") {
			t.Errorf("mlab=%v: aviso/atribución incorrectos", mlab)
		}
		for _, want := range []string{"10.0.0.1", "10.0.0.2", "@page", "A4", "@media print", "Cliente", "Técnico", "Notas"} {
			if !strings.Contains(s, want) {
				t.Errorf("falta %q", want)
			}
		}
		if strings.Contains(low, "prefers-color-scheme: dark") {
			t.Error("el informe usa tema claro")
		}
	}
}

func TestReportEmpty(t *testing.T) {
	b, err := ReportHTML(nil, ReportOptions{}, reportNow)
	if err != nil || !strings.Contains(string(b), "<html") {
		t.Fatalf("informe vacío: %v", err)
	}
	if strings.Contains(string(b), "Cliente:") {
		t.Error("sin cliente no debe mostrarse la línea")
	}
}

func TestReportInternetGolden(t *testing.T) {
	recs := []*history.Record{{ID: "20260305-102000-bbbbbbbb", Time: "2026-03-05T10:20:00+01:00", Kind: "internet", Status: "done",
		Dest:   &history.Endpoint{Alias: "1.1.1.1", IP: "1.1.1.1"},
		Result: []byte(`{"loss_pct":0,"min_ms":11,"avg_ms":14.2,"max_ms":21,"jitter_ms":2.1}`)}}
	b, err := ReportHTML(recs, ReportOptions{}, reportNow)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "Wi-Fi (SSID)") || strings.Contains(string(b), "<th>Enlace</th>") {
		t.Error("columnas Wi-Fi/Enlace sin datos")
	}
	golden(t, "report-internet.html", b)
}
