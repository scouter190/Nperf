package ndt7

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"nperf/internal/ndt7/ndt7test"
	"nperf/internal/version"
)

func locateWith(t *testing.T, mode ndt7test.LocateMode) (*ndt7test.Server, Target, error) {
	t.Helper()
	s, err := ndt7test.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	s.Configure(func(c *ndt7test.Config) { c.Locate = mode })
	tg, err := Locate(context.Background(), LocateOptions{BaseURL: s.URL, TLS: s.TLSConfig()})
	return s, tg, err
}

func wantReason(t *testing.T, err error, sub string) {
	t.Helper()
	var le *LocateError
	if !errors.As(err, &le) || !strings.Contains(le.Reason, sub) {
		t.Fatalf("error = %v, quiero motivo con %q", err, sub)
	}
}

func TestLocateOK(t *testing.T) {
	s, tg, err := locateWith(t, ndt7test.LocateOK)
	if err != nil {
		t.Fatal(err)
	}
	q, ua, path := s.LastLocate()
	if path != "/v2/nearest/ndt/ndt7" || q.Get("client_name") != "nperf" || q.Get("client_version") != version.Current {
		t.Fatalf("petición: %s %v", path, q)
	}
	if ua != "NPerf/"+version.Current {
		t.Fatalf("UA %q", ua)
	}
	if tg.City != "Pruebaville" || tg.Country != "ZZ" || tg.Machine == "" {
		t.Fatalf("target %+v", tg)
	}
	wantDL := "wss://" + s.Addr() + "/ndt/v7/download?access_token=" + ndt7test.Token + "&index=0"
	wantUL := "wss://" + s.Addr() + "/ndt/v7/upload?access_token=" + ndt7test.Token + "&index=0"
	if tg.DownloadURL != wantDL || tg.UploadURL != wantUL {
		t.Fatalf("URLs alteradas: %q %q", tg.DownloadURL, tg.UploadURL)
	}
}

func TestLocateFailures(t *testing.T) {
	for _, c := range []struct {
		mode ndt7test.LocateMode
		sub  string
	}{
		{ndt7test.LocateEmpty, "no tiene servidores"},
		{ndt7test.Locate500, "HTTP 500"},
		{ndt7test.Locate429, "M-Lab limita las pruebas ahora mismo"},
		{ndt7test.LocateBadJSON, "fuera de protocolo"},
		{ndt7test.LocateBadScheme, "fuera de protocolo"},
	} {
		_, _, err := locateWith(t, c.mode)
		wantReason(t, err, c.sub)
	}
}

func TestLocateRefused(t *testing.T) {
	s, err := ndt7test.New()
	if err != nil {
		t.Fatal(err)
	}
	base := s.URL
	s.Close()
	_, err = Locate(context.Background(), LocateOptions{BaseURL: base, TLS: s.TLSConfig(), Timeout: 3 * time.Second})
	wantReason(t, err, "no se pudo contactar")
}
