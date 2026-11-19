package history

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func sample(kind string) *Record {
	return &Record{
		FormatVersion: 1, ID: "20260102-030405-0a1b2c3d", Time: "2026-01-02T04:04:05+01:00",
		Kind: kind, Status: StatusDone, Version: "1.0",
		Source:    &Endpoint{Alias: "A", IP: "10.0.0.1", SSID: "casa", Link: "wifi", NominalMbps: 866.7},
		Dest:      &Endpoint{Alias: "B", IP: "10.0.0.2"},
		Params:    json.RawMessage(`{"duration_s":10}`),
		Result:    json.RawMessage(`{"a":[1,2]}`),
		Diagnosis: json.RawMessage(`{"x":"y"}`),
		Chart:     json.RawMessage(`{"n":1}`),
		Notes:     []string{"nota"},
	}
}

func TestRoundTripKinds(t *testing.T) {
	for _, k := range []string{KindTest, KindPing, KindTrace, KindMTU, KindPorts, KindInternet, KindSpeed} {
		in := sample(k)
		b, err := in.Encode()
		if err != nil {
			t.Fatal(k, err)
		}
		out, err := Decode(b)
		if err != nil {
			t.Fatal(k, err)
		}
		if !reflect.DeepEqual(in, out) {
			t.Fatalf("%s: %+v != %+v", k, in, out)
		}
	}
}

func TestNewIDAndValid(t *testing.T) {
	id, err := NewID(time.Date(2026, 1, 2, 4, 4, 5, 0, time.FixedZone("x", 3600)), bytes.NewReader([]byte{0xde, 0xad, 0xbe, 0xef}))
	if err != nil || id != "20260102-030405-deadbeef" || !ValidID(id) {
		t.Fatal(id, err)
	}
	if _, err := NewID(time.Now(), bytes.NewReader(nil)); err == nil {
		t.Fatal("esperaba error")
	}
	for _, bad := range []string{"", "..", "a/b", `a\b`, "../20260102-030405-deadbeef", "20260102-030405-deadbeef/", "20260102-030405-DEADBEEF", strings.Repeat("a", 500), "20260102-030405-deadbeef\n"} {
		if ValidID(bad) {
			t.Fatalf("id aceptado: %q", bad)
		}
	}
}

func TestDecodeRejects(t *testing.T) {
	good, _ := sample(KindTest).Encode()
	cases := map[string]string{
		"vacío":      ``,
		"tipo":       `{"format_version":"1"}`,
		"kind":       strings.Replace(string(good), `"kind":"test"`, `"kind":"otro"`, 1),
		"status":     strings.Replace(string(good), `"status":"done"`, `"status":"x"`, 1),
		"id":         strings.Replace(string(good), `20260102-030405-0a1b2c3d`, `../x`, 1),
		"time":       strings.Replace(string(good), `2026-01-02T04:04:05+01:00`, `ayer`, 1),
		"params":     strings.Replace(string(good), `{"duration_s":10}`, `[1]`, 1),
		"nominal":    strings.Replace(string(good), `866.7`, `"rápido"`, 1),
		"notes tipo": strings.Replace(string(good), `["nota"]`, `[1]`, 1),
		"extra":      string(good) + `{}`,
	}
	for n, s := range cases {
		if _, err := Decode([]byte(s)); err == nil {
			t.Errorf("%s: aceptado", n)
		}
	}
	big := sample(KindTest)
	big.Result = json.RawMessage(`{"x":"` + strings.Repeat("a", MaxRecordBytes) + `"}`)
	if _, err := big.Encode(); err == nil {
		t.Error("Encode > 2 MB aceptado")
	}
	b, _ := json.Marshal(big)
	if _, err := Decode(b); err == nil {
		t.Error("Decode > 2 MB aceptado")
	}
}

func TestNoSecretFields(t *testing.T) {
	b, _ := sample(KindTest).Encode()
	s := strings.ToLower(string(b))
	for _, w := range []string{"key", "mac", "bssid"} {
		if strings.Contains(s, `"`+w) {
			t.Errorf("campo %s presente", w)
		}
	}
}
