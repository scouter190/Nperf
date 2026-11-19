package ndt7

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"nperf/internal/version"
)

// DefaultLocateBase es la base de Locate v2 de M-Lab.
const DefaultLocateBase = "https://locate.measurementlab.net"

const (
	locateTimeout = 10 * time.Second
	locateMaxBody = 64 << 10
	urlDownload   = "wss:///ndt/v7/download"
	urlUpload     = "wss:///ndt/v7/upload"
)

// LocateOptions configura la consulta; todos los campos son opcionales.
type LocateOptions struct {
	BaseURL string      // por defecto DefaultLocateBase
	TLS     *tls.Config // para pruebas (CA inyectada)
	Version string      // por defecto version.Current
	Timeout time.Duration
}

// Target es el servidor elegido por Locate; las URLs se usan sin modificar.
type Target struct {
	Machine     string
	City        string
	Country     string
	DownloadURL string
	UploadURL   string
}

// LocateError lleva el motivo legible (en español) de un fallo de Locate.
type LocateError struct {
	Reason string
	Err    error
}

func (e *LocateError) Error() string { return e.Reason }
func (e *LocateError) Unwrap() error { return e.Err }

func locateFail(reason string, err error) error { return &LocateError{Reason: reason, Err: err} }

// Locate consulta Locate v2 y devuelve el primer resultado.
func Locate(ctx context.Context, o LocateOptions) (Target, error) {
	base := o.BaseURL
	if base == "" {
		base = DefaultLocateBase
	}
	ver := o.Version
	if ver == "" {
		ver = version.Current
	}
	timeout := o.Timeout
	if timeout <= 0 {
		timeout = locateTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	q := url.Values{"client_name": {"nperf"}, "client_version": {ver}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v2/nearest/ndt/ndt7?"+q.Encode(), nil)
	if err != nil {
		return Target{}, locateFail("dirección de M-Lab inválida", err)
	}
	req.Header.Set("User-Agent", "NPerf/"+ver)
	tr := &http.Transport{Proxy: nil, TLSClientConfig: o.TLS, ForceAttemptHTTP2: false, DisableKeepAlives: true}
	defer tr.CloseIdleConnections()
	resp, err := (&http.Client{Transport: tr, Timeout: timeout}).Do(req)
	if err != nil {
		if ctx.Err() != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Target{}, locateFail("M-Lab no respondió a tiempo", err)
		}
		if ctx.Err() != nil {
			return Target{}, locateFail("consulta a M-Lab cancelada", err)
		}
		return Target{}, locateFail("no se pudo contactar con M-Lab", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return Target{}, locateFail("M-Lab limita las pruebas ahora mismo", nil)
	case resp.StatusCode != http.StatusOK:
		return Target{}, locateFail(fmt.Sprintf("M-Lab respondió con error HTTP %d", resp.StatusCode), nil)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, locateMaxBody+1))
	if err != nil {
		return Target{}, locateFail("no se pudo leer la respuesta de M-Lab", err)
	}
	if len(body) > locateMaxBody {
		return Target{}, locateFail("respuesta de M-Lab fuera de protocolo", nil)
	}
	var v struct {
		Results []struct {
			Machine  string            `json:"machine"`
			Location map[string]string `json:"location"`
			URLs     map[string]string `json:"urls"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return Target{}, locateFail("respuesta de M-Lab fuera de protocolo", err)
	}
	if len(v.Results) == 0 {
		return Target{}, locateFail("M-Lab no tiene servidores disponibles", nil)
	}
	r := v.Results[0]
	t := Target{
		Machine: r.Machine, City: r.Location["city"], Country: r.Location["country"],
		DownloadURL: r.URLs[urlDownload], UploadURL: r.URLs[urlUpload],
	}
	for _, s := range []string{t.DownloadURL, t.UploadURL} {
		u, err := url.Parse(s)
		if s == "" || err != nil || u.Scheme != "wss" || u.Host == "" {
			return Target{}, locateFail("respuesta de M-Lab fuera de protocolo", nil)
		}
	}
	return t, nil
}
