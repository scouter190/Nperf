package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"nperf/internal/probe"
)

type fakeEcho struct {
	calls atomic.Int32
	reply probe.Reply
	err   error
}

func (f *fakeEcho) Echo(_ context.Context, r probe.EchoRequest) (probe.Reply, error) {
	f.calls.Add(1)
	rep := f.reply
	if r.TTL > 0 && r.TTL < 2 {
		return probe.Reply{Status: probe.StatusTTLExpired, From: netip.MustParseAddr("192.168.1.1"), RTTms: 1}, f.err
	}
	return rep, f.err
}

type fakeRes struct {
	ip  netip.Addr
	err error
}

func (f fakeRes) LookupIP(context.Context, string) ([]netip.Addr, error) {
	if f.err != nil {
		return nil, f.err
	}
	return []netip.Addr{f.ip}, nil
}

type refuseDialer struct{}

func (refuseDialer) DialContext(context.Context, string, string) (net.Conn, error) {
	return nil, errors.New("connectex: refused")
}

func useProbe(t *testing.T, e *fakeEcho, res fakeRes) (*bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	old, so, se := probeSystem, stdout, stderr
	out, errb := &bytes.Buffer{}, &bytes.Buffer{}
	stdout, stderr = out, errb
	probeSystem = func() (probe.Env, func()) {
		return probe.Env{Echoer: e, Resolver: res, Clock: probe.RealClock{}, Dialer: refuseDialer{}}, func() {}
	}
	t.Cleanup(func() { probeSystem, stdout, stderr = old, so, se })
	return out, errb
}

var okEcho = func() *fakeEcho {
	return &fakeEcho{reply: probe.Reply{Status: probe.StatusOK, From: netip.MustParseAddr("8.8.8.8"), RTTms: 5}}
}

var google = fakeRes{ip: netip.MustParseAddr("8.8.8.8")}

func TestProbePingText(t *testing.T) {
	out, _ := useProbe(t, okEcho(), google)
	if c := run([]string{"ping", "8.8.8.8", "-count", "2", "-interval", "200ms"}); c != 0 {
		t.Fatalf("código %d", c)
	}
	if !strings.Contains(out.String(), "8.8.8.8") {
		t.Fatalf("salida: %q", out.String())
	}
}

func TestProbePingJSON(t *testing.T) {
	out, _ := useProbe(t, okEcho(), google)
	if c := run([]string{"ping", "-count", "1", "-json", "8.8.8.8"}); c != 0 {
		t.Fatalf("código %d", c)
	}
	var m map[string]any
	if err := json.Unmarshal(out.Bytes(), &m); err != nil {
		t.Fatal(err, out.String())
	}
	if m["command"] != "ping" || m["format_version"] != float64(1) {
		t.Fatalf("json: %v", m)
	}
}

func TestProbeTraceMTUPortsOutput(t *testing.T) {
	for _, args := range [][]string{
		{"trace", "8.8.8.8", "-max-hops", "3", "-json"},
		{"mtu", "8.8.8.8", "-json"},
		{"ports", "8.8.8.8", "-p", "22,80", "-json"},
		{"trace", "8.8.8.8", "-max-hops", "3"},
		{"mtu", "8.8.8.8"},
		{"ports", "8.8.8.8", "-p", "22"},
	} {
		out, _ := useProbe(t, okEcho(), google)
		if c := run(args); c != 0 {
			t.Fatalf("%v: código %d", args, c)
		}
		if strings.Contains(strings.Join(args, " "), "-json") {
			var m map[string]any
			if err := json.Unmarshal(out.Bytes(), &m); err != nil || m["command"] != args[0] {
				t.Fatalf("%v: json %v %q", args, err, out.String())
			}
		} else if out.Len() == 0 {
			t.Fatalf("%v: sin salida", args)
		}
	}
}

func TestProbeUnresolvedIsError(t *testing.T) {
	e := okEcho()
	_, errb := useProbe(t, e, fakeRes{err: errors.New("no such host")})
	if c := run([]string{"ping", "no.existe.example"}); c != 1 {
		t.Fatalf("código %d", c)
	}
	if !strings.Contains(errb.String(), "error:") || e.calls.Load() != 0 {
		t.Fatalf("%q", errb.String())
	}
}

func TestProbeTotalLossIsError(t *testing.T) {
	e := &fakeEcho{reply: probe.Reply{Status: probe.StatusTimeout}}
	useProbe(t, e, google)
	if c := run([]string{"ping", "8.8.8.8", "-count", "1"}); c != 1 {
		t.Fatalf("código %d", c)
	}
}

func TestProbeEchoFailureIsError(t *testing.T) {
	e := &fakeEcho{err: errors.New("IcmpSendEcho falló")}
	out, _ := useProbe(t, e, google)
	if c := run([]string{"ping", "8.8.8.8", "-count", "1", "-json"}); c != 1 {
		t.Fatalf("código %d", c)
	}
	if !strings.Contains(out.String(), `"error"`) {
		t.Fatalf("%q", out.String())
	}
}

func TestProbeUsageErrorsNeverEcho(t *testing.T) {
	for _, args := range [][]string{
		{"ping", "8.8.8.8", "-bogus"},
		{"ping", "8.8.8.8", "-count", "0"},
		{"ping", "8.8.8.8", "-count", "101"},
		{"ping", "8.8.8.8", "-interval", "100ms"},
		{"ping", "8.8.8.8", "-timeout", "6s"},
		{"trace", "8.8.8.8", "-max-hops", "65"},
		{"trace", "8.8.8.8", "-timeout", "100ms"},
		{"mtu", "8.8.8.8", "-timeout", "10s"},
		{"mtu", "8.8.8.8", "-x"},
		{"ports", "8.8.8.8", "-p", "0"},
		{"ports", "8.8.8.8", "-p", "1-40"},
		{"ports", "8.8.8.8", "-timeout", "100ms"},
		{"ports", "8.8.8.8", "-timeout", "6s"},
		{"ping"},
		{"ping", "http://x"},
		{"ping", "255.255.255.255"},
		{"ping", "8.8.8.8", "9.9.9.9"},
	} {
		e := okEcho()
		useProbe(t, e, google)
		if c := run(args); c != 2 {
			t.Errorf("%v: código %d, quería 2", args, c)
		}
		if e.calls.Load() != 0 {
			t.Errorf("%v: hubo Echo", args)
		}
	}
}

func TestProbeUsageErrorJSON(t *testing.T) {
	out, errb := useProbe(t, okEcho(), google)
	if c := run([]string{"ping", "8.8.8.8", "-count", "0", "-json"}); c != 2 {
		t.Fatalf("código %d", c)
	}
	var m map[string]any
	if json.Unmarshal(out.Bytes(), &m) != nil || m["error"] == nil || errb.Len() != 0 {
		t.Fatalf("%q %q", out.String(), errb.String())
	}
}

func TestProbeHelpSpanish(t *testing.T) {
	for _, c := range []string{"ping", "trace", "mtu", "ports"} {
		out, _ := useProbe(t, okEcho(), google)
		if code := run([]string{c, "-h"}); code != 0 || !strings.Contains(out.String(), "Uso: nperf "+c) {
			t.Fatalf("%s: %d %q", c, code, out.String())
		}
	}
}

func TestProbeCanceled(t *testing.T) {
	e := okEcho()
	_, errb := useProbe(t, e, google)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if c := runProbe(ctx, "ping", []string{"8.8.8.8", "-count", "3"}); c != 1 {
		t.Fatalf("código %d", c)
	}
	if !strings.Contains(errb.String(), "cancelado") {
		t.Fatalf("%q", errb.String())
	}
	_ = time.Second
}
