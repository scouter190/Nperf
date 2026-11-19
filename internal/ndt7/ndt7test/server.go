// Package ndt7test es un servidor NDT7 y Locate v2 local, solo para tests:
// HTTPS con una CA generada en memoria (exportada para RootCAs), sin red
// externa. Lo usan los tests de ndt7, app y cmd/nperf.
package ndt7test

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"time"

	"nperf/internal/ws"
)

// Subprotocol es el subprotocolo WebSocket de ndt7.
const Subprotocol = "net.measurementlab.ndt.v7"

// Token es el access_token que el servidor exige en las URLs de medición.
const Token = "tok-test-123"

// LocateMode es el comportamiento del endpoint de Locate.
type LocateMode int

const (
	LocateOK        LocateMode = iota // un resultado con URLs wss
	LocateEmpty                       // {"results":[]}
	Locate500                         // HTTP 500
	Locate429                         // HTTP 429
	LocateBadJSON                     // cuerpo que no es JSON
	LocateBadScheme                   // URLs ws:// en lugar de wss://
)

// FailMode es el comportamiento de los endpoints de medición.
type FailMode int

const (
	Normal           FailMode = iota // cierra con 1000 al terminar
	NoClose                          // al terminar no cierra ni envía más
	LateClose                        // cierra pasado Config.LateClose (≈ 13 s)
	CutMidway                        // corta el TCP a la mitad
	GiantFrame                       // primer mensaje de GiantSize bytes
	InvalidJSON                      // primer mensaje de texto no es JSON
	WrongSubprotocol                 // handshake sin el subprotocolo pedido
)

// Config se modifica con Server.Configure.
type Config struct {
	Locate          LocateMode
	Fail            FailMode
	DownloadRate    float64       // bytes/s enviados en la descarga
	DownloadTime    time.Duration // duración normal de la descarga
	UploadTime      time.Duration // el servidor cierra la subida a este tiempo
	LateClose       time.Duration // instante de cierre en modo LateClose
	MeasureInterval time.Duration
	GiantSize       int
	MinRTT          time.Duration
}

// Stats son los contadores.
type Stats struct {
	Requests       int // peticiones HTTP de cualquier ruta
	LocateRequests int
	Connections    int // WebSockets aceptados
	DownloadConns  int
	UploadConns    int
	BytesSent      int64 // bytes binarios enviados en descargas
	BytesReceived  int64 // bytes binarios recibidos en subidas
}

// Server es el servidor de prueba.
type Server struct {
	// URL base HTTPS (para el locate: {URL}/v2/nearest/ndt/ndt7).
	URL string
	// CA es la CA generada, lista para tls.Config.RootCAs.
	CA *x509.CertPool
	// CAPEM es la misma CA en PEM.
	CAPEM []byte

	ts     *httptest.Server
	ctx    context.Context
	cancel context.CancelFunc

	mu          sync.Mutex
	cfg         Config
	stats       Stats
	lastQuery   url.Values
	lastUA      string
	lastPath    string
	lastProxyOK bool
}

// New arranca el servidor; llamar a Close al terminar.
func New() (*Server, error) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "ndt7test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return nil, err
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		DNSNames:     []string{"localhost"},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	pool.AddCert(caCert)

	s := &Server{
		CA:    pool,
		CAPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
		cfg: Config{
			DownloadRate:    5e6,
			DownloadTime:    10 * time.Second,
			UploadTime:      10 * time.Second,
			LateClose:       13 * time.Second,
			MeasureInterval: 100 * time.Millisecond,
			GiantSize:       ws.DefaultMaxMessage + 1<<20,
			MinRTT:          12 * time.Millisecond,
		},
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())

	mux := http.NewServeMux()
	mux.HandleFunc("/v2/nearest/ndt/ndt7", s.locate)
	mux.HandleFunc("/ndt/v7/download", s.download)
	mux.HandleFunc("/ndt/v7/upload", s.upload)
	s.ts = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.stats.Requests++
		s.mu.Unlock()
		mux.ServeHTTP(w, r)
	}))
	s.ts.TLS = &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{leafDER}, PrivateKey: leafKey}},
		NextProtos:   []string{"http/1.1"},
	}
	s.ts.StartTLS()
	s.URL = s.ts.URL
	return s, nil
}

// Close detiene el servidor y corta las conexiones abiertas.
func (s *Server) Close() {
	s.cancel()
	s.ts.Close()
}

// Addr devuelve host:puerto.
func (s *Server) Addr() string { return s.ts.Listener.Addr().String() }

// TLSConfig devuelve una configuración de cliente que confía en la CA.
func (s *Server) TLSConfig() *tls.Config {
	return &tls.Config{RootCAs: s.CA, MinVersion: tls.VersionTLS12}
}

// Configure modifica la configuración de forma segura.
func (s *Server) Configure(f func(*Config)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(&s.cfg)
}

// Stats devuelve una copia de los contadores.
func (s *Server) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stats
}

// LastLocate devuelve parámetros, User-Agent y ruta de la última petición de locate.
func (s *Server) LastLocate() (q url.Values, userAgent, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastQuery, s.lastUA, s.lastPath
}

func (s *Server) snapshot() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

func (s *Server) locate(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.stats.LocateRequests++
	s.lastQuery, s.lastUA, s.lastPath = r.URL.Query(), r.UserAgent(), r.URL.Path
	cfg := s.cfg
	s.mu.Unlock()

	switch cfg.Locate {
	case Locate500:
		http.Error(w, "error interno", http.StatusInternalServerError)
		return
	case Locate429:
		http.Error(w, "demasiadas peticiones", http.StatusTooManyRequests)
		return
	case LocateBadJSON:
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "{no es json")
		return
	}
	type result struct {
		Machine  string            `json:"machine"`
		Location map[string]string `json:"location"`
		URLs     map[string]string `json:"urls"`
	}
	out := struct {
		Results []result `json:"results"`
	}{Results: []result{}}
	if cfg.Locate != LocateEmpty {
		scheme := "wss"
		if cfg.Locate == LocateBadScheme {
			scheme = "ws"
		}
		q := "?access_token=" + Token + "&index=0"
		base := scheme + "://" + s.Addr()
		out.Results = append(out.Results, result{
			Machine:  "mlab1-test01.mlab-sandbox.measurement-lab.org",
			Location: map[string]string{"city": "Pruebaville", "country": "ZZ"},
			URLs: map[string]string{
				"wss:///ndt/v7/download": base + "/ndt/v7/download" + q,
				"wss:///ndt/v7/upload":   base + "/ndt/v7/upload" + q,
			},
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// hijackSpy captura la net.Conn para poder cortarla en seco.
type hijackSpy struct {
	http.ResponseWriter
	conn net.Conn
}

func (h *hijackSpy) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := h.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("sin Hijack")
	}
	c, rw, err := hj.Hijack()
	h.conn = c
	return c, rw, err
}

// accept valida el token y hace el handshake; devuelve nil si ya respondió.
func (s *Server) accept(w http.ResponseWriter, r *http.Request, cfg Config, upload bool) (*ws.Conn, net.Conn, context.Context, context.CancelFunc) {
	if r.URL.Query().Get("access_token") != Token {
		http.Error(w, "token inválido", http.StatusUnauthorized)
		return nil, nil, nil, nil
	}
	opts := ws.Options{Subprotocol: Subprotocol, MaxMessage: int64(cfg.GiantSize) * 2}
	if cfg.Fail == WrongSubprotocol {
		opts.Subprotocol = ""
	}
	spy := &hijackSpy{ResponseWriter: w}
	c, err := ws.Accept(spy, r, opts)
	if err != nil {
		return nil, nil, nil, nil
	}
	s.mu.Lock()
	s.stats.Connections++
	if upload {
		s.stats.UploadConns++
	} else {
		s.stats.DownloadConns++
	}
	s.mu.Unlock()
	ctx, cancel := context.WithCancel(s.ctx)
	stop := context.AfterFunc(ctx, func() { _ = spy.conn.Close() })
	return c, spy.conn, ctx, func() { stop(); cancel() }
}

type measurement struct {
	AppInfo struct {
		ElapsedTime int64
		NumBytes    int64
	}
	TCPInfo struct {
		ElapsedTime   int64
		BytesReceived int64 `json:",omitempty"`
		BytesAcked    int64 `json:",omitempty"`
		MinRTT        int64
	}
}

func (s *Server) measurementJSON(cfg Config, since time.Time, n int64, upload bool) []byte {
	var m measurement
	el := time.Since(since).Microseconds()
	m.AppInfo.ElapsedTime, m.AppInfo.NumBytes = el, n
	m.TCPInfo.ElapsedTime = el
	m.TCPInfo.MinRTT = cfg.MinRTT.Microseconds()
	if upload {
		m.TCPInfo.BytesReceived = n
	} else {
		m.TCPInfo.BytesAcked = n
	}
	b, _ := json.Marshal(m)
	return b
}

// drain lee y descarta hasta que la conexión termina; cierra done.
func drain(ctx context.Context, c *ws.Conn, done chan<- struct{}) {
	defer close(done)
	for {
		if _, _, err := c.ReadMessage(ctx); err != nil {
			return
		}
	}
}

func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	cfg := s.snapshot()
	c, raw, ctx, release := s.accept(w, r, cfg, false)
	if c == nil {
		return
	}
	defer release()
	if cfg.Fail == WrongSubprotocol {
		return // el cliente rechaza el handshake y cierra
	}
	done := make(chan struct{})
	dctx, dcancel := context.WithCancel(ctx)
	defer dcancel()
	go drain(dctx, c, done)

	switch cfg.Fail {
	case GiantFrame:
		_ = c.WriteMessage(ctx, ws.Binary, make([]byte, cfg.GiantSize))
		<-done
		return
	case InvalidJSON:
		_ = c.WriteMessage(ctx, ws.Text, []byte("{esto no es json"))
	}

	total := cfg.DownloadTime
	if cfg.Fail == LateClose {
		total = cfg.LateClose
	}
	chunk := make([]byte, 8192)
	_, _ = rand.Read(chunk)
	start := time.Now()
	lastM := start
	var sent int64
	cut := false
	for {
		el := time.Since(start)
		if cfg.Fail == CutMidway && el >= cfg.DownloadTime/2 {
			cut = true
			break
		}
		if el >= total || (cfg.Fail == NoClose && el >= cfg.DownloadTime) {
			break
		}
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		default:
		}
		if float64(sent) > cfg.DownloadRate*el.Seconds() {
			time.Sleep(2 * time.Millisecond)
		} else {
			if err := c.WriteMessage(ctx, ws.Binary, chunk); err != nil {
				return
			}
			sent += int64(len(chunk))
			s.mu.Lock()
			s.stats.BytesSent += int64(len(chunk))
			s.mu.Unlock()
		}
		if time.Since(lastM) >= cfg.MeasureInterval {
			lastM = time.Now()
			if err := c.WriteMessage(ctx, ws.Text, s.measurementJSON(cfg, start, sent, false)); err != nil {
				return
			}
		}
	}
	switch {
	case cut:
		_ = raw.Close()
	case cfg.Fail == NoClose:
		select { // ni cierra ni envía: espera al cliente
		case <-done:
		case <-ctx.Done():
		}
	default:
		_ = c.WriteMessage(ctx, ws.Text, s.measurementJSON(cfg, start, sent, false))
		dcancel() // un solo lector: Close drena por su cuenta
		<-done
		_ = c.Close(ws.CloseNormal, "")
	}
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	cfg := s.snapshot()
	c, raw, ctx, release := s.accept(w, r, cfg, true)
	if c == nil {
		return
	}
	defer release()
	if cfg.Fail == WrongSubprotocol {
		return
	}
	switch cfg.Fail {
	case GiantFrame:
		_ = c.WriteMessage(ctx, ws.Binary, make([]byte, cfg.GiantSize))
	case InvalidJSON:
		_ = c.WriteMessage(ctx, ws.Text, []byte("{esto no es json"))
	}

	start := time.Now()
	var recv int64
	var rmu sync.Mutex
	get := func() int64 { rmu.Lock(); defer rmu.Unlock(); return recv }

	stopM := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(cfg.MeasureInterval)
		defer t.Stop()
		for {
			select {
			case <-stopM:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				if c.WriteMessage(ctx, ws.Text, s.measurementJSON(cfg, start, get(), true)) != nil {
					return
				}
			}
		}
	}()
	finish := func() { close(stopM); wg.Wait() }

	deadline := start.Add(cfg.UploadTime)
	rctx, rcancel := context.WithDeadline(ctx, deadline)
	defer rcancel()
	for {
		if cfg.Fail == CutMidway && time.Since(start) >= cfg.UploadTime/2 {
			finish()
			_ = raw.Close()
			return
		}
		typ, data, err := c.ReadMessage(rctx)
		if err != nil {
			finish()
			if rctx.Err() != nil && ctx.Err() == nil && cfg.Fail != NoClose {
				_ = c.WriteMessage(ctx, ws.Text, s.measurementJSON(cfg, start, get(), true))
				_ = c.Close(ws.CloseNormal, "")
			} else if rctx.Err() != nil && ctx.Err() == nil { // NoClose: espera al cliente
				<-ctx.Done()
			}
			return
		}
		if typ == ws.Binary {
			rmu.Lock()
			recv += int64(len(data))
			rmu.Unlock()
			s.mu.Lock()
			s.stats.BytesReceived += int64(len(data))
			s.mu.Unlock()
		}
	}
}
