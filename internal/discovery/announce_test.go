package discovery

import (
	"net/netip"
	"strings"
	"testing"

	"nperf/internal/netinfo"
)

func sample() Announce {
	id, _ := NewInstanceID()
	return Announce{Alias: "Sala 1", IP: netip.MustParseAddr("192.168.1.20"), Port: 5300,
		State: StateAvailable, Queue: 2, InstanceID: id}
}

func TestRoundTrip(t *testing.T) {
	for _, alias := range []string{"Sala 1", "Cañón de la oficina ñandú ✓", "", "a=b c%d"} {
		for _, ip := range []string{"192.168.1.20", "10.0.0.1"} {
			a := sample()
			a.Alias, a.IP, a.State = alias, netip.MustParseAddr(ip), StateBusy
			b, err := a.Marshal()
			if err != nil {
				t.Fatal(err)
			}
			got, err := Parse(b)
			if err != nil || got != a {
				t.Fatalf("got %+v err %v, want %+v", got, err, a)
			}
		}
	}
}

func TestLongAliasTruncated(t *testing.T) {
	a := sample()
	a.Alias = strings.Repeat("ñ", 1000)
	b, err := a.Marshal()
	if err != nil || len(b) > MaxSize {
		t.Fatalf("len %d err %v", len(b), err)
	}
	got, err := Parse(b)
	if err != nil || got.Alias == "" || !strings.HasPrefix(a.Alias, got.Alias) {
		t.Fatalf("alias truncado inválido: %v", err)
	}
}

func TestMarshalInvalid(t *testing.T) {
	cases := map[string]func(*Announce){
		"ip":    func(a *Announce) { a.IP = netip.Addr{} },
		"port0": func(a *Announce) { a.Port = 0 },
		"port+": func(a *Announce) { a.Port = 70000 },
		"state": func(a *Announce) { a.State = "x" },
		"queue": func(a *Announce) { a.Queue = -1 },
		"id":    func(a *Announce) { a.InstanceID = "zz" },
		"utf8":  func(a *Announce) { a.Alias = "\xff" },
	}
	for n, f := range cases {
		a := sample()
		f(&a)
		if _, err := a.Marshal(); err == nil {
			t.Errorf("%s: se esperaba error", n)
		}
	}
}

func TestParseRejects(t *testing.T) {
	good, _ := sample().Marshal()
	g := string(good)
	cases := map[string]string{
		"firma":      strings.Replace(g, "NPDISC1", "NPDISC2", 1),
		"vacío":      "",
		"truncado":   g[:len(g)/2],
		"sin alias":  strings.Replace(g, " alias=Sala%201", "", 1),
		"puerto":     strings.Replace(g, "port=5300", "port=70000", 1),
		"puerto0":    strings.Replace(g, "port=5300", "port=0", 1),
		"puerto txt": strings.Replace(g, "port=5300", "port=abc", 1),
		"estado":     strings.Replace(g, "state=available", "state=roto", 1),
		"cola":       strings.Replace(g, "queue=2", "queue=-1", 1),
		"ip":         strings.Replace(g, "ip=192.168.1.20", "ip=999.1.1.1", 1),
		"id":         strings.Replace(g, "id=", "id=xx", 1),
		"repetido":   g + " port=1",
		"alias malo": g + "%zz",
		"ctl":        g + "%0A",
		"v6":         strings.Replace(g, "ip=192.168.1.20", "ip=fe80::1", 1),
		"grande":     g + strings.Repeat("a", MaxSize),
	}
	for n, s := range cases {
		if _, err := Parse([]byte(s)); err == nil {
			t.Errorf("%s: se esperaba error", n)
		}
	}
}

func TestParseIgnoresUnknownKeys(t *testing.T) {
	b, _ := sample().Marshal()
	if _, err := Parse(append(b, " future=1"...)); err != nil {
		t.Fatal(err)
	}
}

func TestNoSensitiveData(t *testing.T) {
	a := sample()
	b, _ := a.Marshal()
	s := strings.ToLower(string(b))
	for _, bad := range []string{"key", "clave", "version", "session", "sesion", "sesión", "hmac", "secret"} {
		if strings.Contains(s, bad) {
			t.Errorf("el anuncio contiene %q: %s", bad, s)
		}
	}
	if !strings.HasPrefix(s, "npdisc1 ") {
		t.Error("firma")
	}
}

func TestInstanceIDRandom(t *testing.T) {
	a, _ := NewInstanceID()
	b, _ := NewInstanceID()
	if a == b || !validInstanceID(a) {
		t.Fatal(a, b)
	}
}

func FuzzParse(f *testing.F) {
	good, _ := sample().Marshal()
	f.Add(good)
	f.Add([]byte("NPDISC1"))
	f.Add([]byte("NPDISC1 id== ip= port= state= queue= alias=%"))
	f.Fuzz(func(t *testing.T, data []byte) {
		a, err := Parse(data)
		if err == nil {
			if _, err := a.Marshal(); err != nil {
				t.Fatalf("Parse válido no re-serializa: %v", err)
			}
		}
	})
}

func TestExact512(t *testing.T) {
	a := sample()
	a.Alias = ""
	b, _ := a.Marshal()
	e := MaxSize - len(b)
	for k := 0; k <= 128; k++ {
		m := e - 6*k
		if m < 0 || 2*k+m > MaxAliasBytes {
			continue
		}
		a.Alias = strings.Repeat("ñ", k) + strings.Repeat("a", m)
		b, err := a.Marshal()
		if err != nil || len(b) != MaxSize {
			t.Fatalf("len %d err %v", len(b), err)
		}
		got, err := Parse(b)
		if err != nil || got.Alias != a.Alias {
			t.Fatal("alias alterado", err)
		}
		return
	}
	t.Fatal("sin combinación")
}

func TestLinkRoundTrip(t *testing.T) {
	a := sample()
	a.Link, a.TxBps, a.RxBps, a.SSID = netinfo.KindWiFi, 866_700_000, 650_000_000, "Café 5G"
	b, err := a.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(b)
	if err != nil || got != a {
		t.Fatalf("got %+v err %v", got, err)
	}
	for _, bad := range []string{"bssid", "mac", "band", "channel", "signal", "standard"} {
		if strings.Contains(string(b), bad) {
			t.Errorf("contiene %q", bad)
		}
	}
}

func TestOldAnnounceUnknownLink(t *testing.T) {
	old := "NPDISC1 id=0123456789abcdef ip=192.168.1.20 port=5300 state=available queue=0 alias=x"
	a, err := Parse([]byte(old))
	if err != nil || a.Link != "" || a.TxBps != 0 || a.RxBps != 0 || a.SSID != "" {
		t.Fatalf("%+v %v", a, err)
	}
}

func TestInvalidSSIDOmittedAndDiscarded(t *testing.T) {
	for _, ssid := range []string{strings.Repeat("a", 33), "a\x01b", "a\nb"} {
		a := sample()
		a.Link, a.SSID = netinfo.KindWiFi, ssid
		b, err := a.Marshal()
		if err != nil || strings.Contains(string(b), "ssid=") {
			t.Fatalf("ssid %q no omitido: %s %v", ssid, b, err)
		}
	}
	good, _ := sample().Marshal()
	for _, ssid := range []string{strings.Repeat("a", 33), "a%01b", "%ff"} {
		a, err := Parse([]byte(string(good) + " ssid=" + ssid))
		if err != nil || a.SSID != "" {
			t.Fatalf("ssid %q no descartado: %+v %v", ssid, a, err)
		}
	}
}

func TestInvalidLinkValuesDiscarded(t *testing.T) {
	good, _ := sample().Marshal()
	a, err := Parse([]byte(string(good) + " link=x tx=-5 rx=2000000000000"))
	if err != nil || a.Link != "" || a.TxBps != 0 || a.RxBps != 0 {
		t.Fatalf("%+v %v", a, err)
	}
	b := sample()
	b.Link, b.TxBps, b.RxBps = "zzz", -1, maxLinkBps+1
	m, _ := b.Marshal()
	for _, k := range []string{"link=", "tx=", "rx="} {
		if strings.Contains(string(m), k) {
			t.Errorf("no omitido %s", k)
		}
	}
}

func TestMaxSizeWithLink(t *testing.T) {
	a := sample()
	a.Alias = strings.Repeat("ñ", 500)
	a.Link, a.TxBps, a.RxBps = netinfo.KindWiFi, maxLinkBps, maxLinkBps
	a.SSID = strings.Repeat("ñ", 16) // 32 B, 96 caracteres codificados
	b, err := a.Marshal()
	if err != nil || len(b) > MaxSize {
		t.Fatalf("len %d err %v", len(b), err)
	}
	got, err := Parse(b)
	if err != nil || got.SSID != a.SSID || got.TxBps != a.TxBps || got.Alias == "" {
		t.Fatalf("%+v %v", got, err)
	}
}
