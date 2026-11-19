package ws

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const sub = "net.measurementlab.ndt.v7"

func start(t *testing.T, h func(c *Conn, r *http.Request)) (string, Options) {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := Accept(w, r, Options{Subprotocol: sub, MaxMessage: 1 << 20})
		if err != nil {
			return
		}
		h(c, r)
	}))
	t.Cleanup(srv.Close)
	return dialOpts(srv)
}

func dialOpts(srv *httptest.Server) (string, Options) {
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	return "wss://" + strings.TrimPrefix(srv.URL, "https://") + "/ndt/v7/download?access_token=a%2Fb",
		Options{TLS: &tls.Config{RootCAs: pool}, Subprotocol: sub, UserAgent: "NPerf/test", MaxMessage: 1 << 20}
}

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return c
}

func TestEchoMaskedAndALPN(t *testing.T) {
	var alpn, ua, uri string
	url, o := start(t, func(c *Conn, r *http.Request) {
		alpn, ua, uri = r.TLS.NegotiatedProtocol, r.UserAgent(), r.RequestURI
		defer c.Close(CloseNormal, "")
		for {
			typ, d, err := c.ReadMessage(context.Background()) // exige máscara
			if err != nil {
				return
			}
			c.WriteMessage(context.Background(), typ, d)
		}
	})
	c, err := Dial(ctx(t), url, o)
	if err != nil {
		t.Fatal(err)
	}
	if c.Subprotocol() != sub {
		t.Fatal("subprotocolo")
	}
	big := []byte(strings.Repeat("x", 70000)) // longitud de 8 bytes
	for _, m := range [][]byte{[]byte("hola"), make([]byte, 300), big} {
		if err := c.WriteMessage(ctx(t), Binary, m); err != nil {
			t.Fatal(err)
		}
		typ, d, err := c.ReadMessage(ctx(t))
		if err != nil || typ != Binary || len(d) != len(m) {
			t.Fatalf("eco: %v %d %d", err, typ, len(d))
		}
	}
	c.Close(CloseNormal, "")
	if alpn != "http/1.1" || ua != "NPerf/test" || !strings.HasSuffix(uri, "access_token=a%2Fb") {
		t.Fatalf("alpn=%q ua=%q uri=%q", alpn, ua, uri)
	}
}

func TestFragmentedAndInterleavedPing(t *testing.T) {
	pong := make(chan string, 1)
	url, o := start(t, func(c *Conn, r *http.Request) {
		c.writeFrameFin(Text, []byte("ab"), false)
		c.writeFrameFin(opPing, []byte("p1"), true) // ping intercalado
		c.writeFrameFin(opCont, []byte("cd"), false)
		c.writeFrameFin(opCont, []byte("ef"), true)
		// el pong llega como trama de control: léelo en crudo
		_, op, p, err := c.readFrame(1 << 10)
		if err == nil && op == opPong {
			pong <- string(p)
		}
		c.writeFrame(Text, []byte("fin"))
		c.Close(CloseNormal, "")
	})
	c, err := Dial(ctx(t), url, o)
	if err != nil {
		t.Fatal(err)
	}
	typ, d, err := c.ReadMessage(ctx(t))
	if err != nil || typ != Text || string(d) != "abcdef" {
		t.Fatalf("%v %d %q", err, typ, d)
	}
	if p := <-pong; p != "p1" {
		t.Fatalf("pong %q", p)
	}
	if _, d, _ := c.ReadMessage(ctx(t)); string(d) != "fin" {
		t.Fatal("fin")
	}
}

func TestCleanCloseEcho(t *testing.T) {
	got := make(chan error, 1)
	url, o := start(t, func(c *Conn, r *http.Request) {
		c.sendClose(1001, "adiós")
		_, _, err := c.ReadMessage(context.Background()) // eco del cliente
		got <- err
		c.c.Close()
	})
	c, err := Dial(ctx(t), url, o)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = c.ReadMessage(ctx(t))
	var ce *CloseError
	if !errors.As(err, &ce) || ce.Code != 1001 || ce.Reason != "adiós" {
		t.Fatalf("cliente: %v", err)
	}
	err = <-got
	if !errors.As(err, &ce) || ce.Code != 1001 {
		t.Fatalf("eco: %v", err)
	}
	c.Close(CloseNormal, "")
}

func TestClientCloseSendsCode(t *testing.T) {
	got := make(chan error, 1)
	url, o := start(t, func(c *Conn, r *http.Request) {
		_, _, err := c.ReadMessage(context.Background())
		got <- err
		c.c.Close()
	})
	c, err := Dial(ctx(t), url, o)
	if err != nil {
		t.Fatal(err)
	}
	c.Close(CloseNormal, "listo")
	var ce *CloseError
	if err := <-got; !errors.As(err, &ce) || ce.Code != CloseNormal || ce.Reason != "listo" {
		t.Fatalf("%v", err)
	}
}

func TestProtocolViolations(t *testing.T) {
	cases := map[string]func(c *Conn){
		"rsv": func(c *Conn) {
			c.c.Write([]byte{0xC1, 0x01, 'x'})
		},
		"control largo": func(c *Conn) {
			c.writeFrame(opPing, make([]byte, 126))
		},
		"control fragmentado": func(c *Conn) {
			c.writeFrameFin(opPing, nil, false)
		},
		"opcode desconocido": func(c *Conn) {
			c.c.Write([]byte{0x83, 0x00})
		},
		"continuación huérfana": func(c *Conn) {
			c.writeFrame(opCont, []byte("x"))
		},
		"trama enmascarada del servidor": func(c *Conn) {
			c.c.Write([]byte{0x81, 0x81, 1, 2, 3, 4, 'x' ^ 1})
		},
		"UTF-8 inválido": func(c *Conn) {
			c.writeFrame(Text, []byte{0xff, 0xfe})
		},
		"longitud con bit alto": func(c *Conn) {
			c.c.Write([]byte{0x82, 127, 0x80, 0, 0, 0, 0, 0, 0, 0})
		},
		"mayor que el tope": func(c *Conn) {
			c.writeFrame(Binary, make([]byte, 1<<20+1))
		},
		"tope en fragmentos": func(c *Conn) {
			c.writeFrameFin(Binary, make([]byte, 1<<19+1), false)
			c.writeFrame(opCont, make([]byte, 1<<19+1))
		},
	}
	for name, send := range cases {
		t.Run(name, func(t *testing.T) {
			url, o := start(t, func(c *Conn, r *http.Request) {
				send(c)
				time.Sleep(200 * time.Millisecond)
				c.c.Close()
			})
			c, err := Dial(ctx(t), url, o)
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = c.ReadMessage(ctx(t))
			if err == nil {
				t.Fatal("se esperaba error")
			}
			if name == "mayor que el tope" || name == "tope en fragmentos" {
				if !errors.Is(err, ErrTooBig) {
					t.Fatalf("%v", err)
				}
			}
		})
	}
}

func TestLimitsApplyToClientWrite(t *testing.T) {
	url, o := start(t, func(c *Conn, r *http.Request) { c.ReadMessage(context.Background()) })
	c, err := Dial(ctx(t), url, o)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.WriteMessage(ctx(t), Binary, make([]byte, 1<<20+1)); !errors.Is(err, ErrTooBig) {
		t.Fatalf("%v", err)
	}
	if err := c.WriteMessage(ctx(t), 7, nil); err == nil {
		t.Fatal("tipo inválido")
	}
	c.Close(CloseNormal, "")
}

func TestContextDeadlineAndCancel(t *testing.T) {
	url, o := start(t, func(c *Conn, r *http.Request) { time.Sleep(2 * time.Second) })
	c, err := Dial(ctx(t), url, o)
	if err != nil {
		t.Fatal(err)
	}
	d, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, _, err := c.ReadMessage(d); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("%v", err)
	}
	cc, cancel2 := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel2)
	if _, _, err := c.ReadMessage(cc); !errors.Is(err, context.Canceled) {
		t.Fatalf("%v", err)
	}
}

func TestBadHandshakes(t *testing.T) {
	good := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Upgrade", "websocket")
		w.Header().Set("Connection", "Upgrade")
		w.Header().Set("Sec-WebSocket-Accept", acceptKey(r.Header.Get("Sec-WebSocket-Key")))
		w.Header().Set("Sec-WebSocket-Protocol", sub)
		w.WriteHeader(101)
	}
	mod := func(f func(w http.ResponseWriter, r *http.Request)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) { f(w, r) }
	}
	cases := map[string]http.HandlerFunc{
		"ok":  good,
		"200": mod(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("hola")) }),
		"429": mod(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "límite", 429) }),
		"accept erróneo": mod(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Upgrade", "websocket")
			w.Header().Set("Connection", "Upgrade")
			w.Header().Set("Sec-WebSocket-Accept", "AAAA")
			w.Header().Set("Sec-WebSocket-Protocol", sub)
			w.WriteHeader(101)
		}),
		"subprotocolo erróneo": mod(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Upgrade", "websocket")
			w.Header().Set("Connection", "Upgrade")
			w.Header().Set("Sec-WebSocket-Accept", acceptKey(r.Header.Get("Sec-WebSocket-Key")))
			w.Header().Set("Sec-WebSocket-Protocol", "otro")
			w.WriteHeader(101)
		}),
		"sin subprotocolo": mod(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Upgrade", "websocket")
			w.Header().Set("Connection", "Upgrade")
			w.Header().Set("Sec-WebSocket-Accept", acceptKey(r.Header.Get("Sec-WebSocket-Key")))
			w.WriteHeader(101)
		}),
		"sin Upgrade": mod(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Connection", "Upgrade")
			w.Header().Set("Sec-WebSocket-Accept", acceptKey(r.Header.Get("Sec-WebSocket-Key")))
			w.Header().Set("Sec-WebSocket-Protocol", sub)
			w.WriteHeader(101)
		}),
		"extensiones": mod(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Sec-WebSocket-Extensions", "permessage-deflate")
			good(w, r)
		}),
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewTLSServer(h)
			defer srv.Close()
			url, o := dialOpts(srv)
			c, err := Dial(ctx(t), url, o)
			if name == "ok" {
				if err != nil {
					t.Fatal(err)
				}
				c.c.Close()
				return
			}
			if err == nil {
				t.Fatal("se esperaba error")
			}
			if name == "429" && !strings.Contains(err.Error(), "429") {
				t.Fatalf("%v", err)
			}
		})
	}
}

func TestDialRejects(t *testing.T) {
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	defer srv.Close()
	url, _ := dialOpts(srv)
	// certificado no confiable: verificación normal
	if _, err := Dial(ctx(t), url, Options{Subprotocol: sub}); err == nil {
		t.Fatal("certificado desconocido aceptado")
	}
	for _, u := range []string{"ws://127.0.0.1/x", "https://127.0.0.1/x", "wss:///x", "::"} {
		if _, err := Dial(ctx(t), u, Options{}); err == nil {
			t.Fatalf("%q aceptada", u)
		}
	}
}

func TestAcceptRejectsPlainRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := Accept(w, r, Options{}); err == nil {
			t.Error("se esperaba error")
		}
	}))
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatal(resp.StatusCode)
	}
}

func TestConcurrentReadWriteIndependentContexts(t *testing.T) {
	url, o := start(t, func(c *Conn, r *http.Request) {
		time.Sleep(600 * time.Millisecond)
		c.ReadMessage(context.Background())
		c.c.Close()
	})
	c, err := Dial(ctx(t), url, o)
	if err != nil {
		t.Fatal(err)
	}
	rctx, cancel := context.WithCancel(context.Background())
	rerr := make(chan error, 1)
	go func() { _, _, err := c.ReadMessage(rctx); rerr <- err }()
	time.Sleep(50 * time.Millisecond)
	cancel() // cancelar la lectura no debe afectar a la escritura
	if err := <-rerr; !errors.Is(err, context.Canceled) {
		t.Fatalf("lectura: %v", err)
	}
	// escritura con plazo largo mientras otra lectura con plazo corto vence
	wctx, wcancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer wcancel()
	short, scancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer scancel()
	rerr2 := make(chan error, 1)
	go func() { _, _, err := c.ReadMessage(short); rerr2 <- err }()
	time.Sleep(100 * time.Millisecond)
	if err := c.WriteMessage(wctx, Binary, []byte("x")); err != nil {
		t.Fatalf("escritura afectada: %v", err)
	}
	if err := <-rerr2; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lectura 2: %v", err)
	}
	// y a la inversa: escritura cancelada no afecta a una lectura en curso
	wc2, wcancel2 := context.WithCancel(context.Background())
	wcancel2()
	_ = c.WriteMessage(wc2, Binary, []byte("y"))
	c.Close(CloseNormal, "")
}

func TestInvalidCloseCode(t *testing.T) {
	for name, p := range map[string][]byte{
		"1005":   {0x03, 0xED},
		"1004":   {0x03, 0xEC},
		"2000":   {0x07, 0xD0},
		"999":    {0x03, 0xE7},
		"utf8":   {0x03, 0xE8, 0xff},
		"1 byte": {0x03},
	} {
		t.Run(name, func(t *testing.T) {
			url, o := start(t, func(c *Conn, r *http.Request) {
				c.writeFrame(opClose, p)
				time.Sleep(200 * time.Millisecond)
				c.c.Close()
			})
			c, err := Dial(ctx(t), url, o)
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = c.ReadMessage(ctx(t))
			var ce *CloseError
			if err == nil || errors.As(err, &ce) {
				t.Fatalf("debía ser error de protocolo: %v", err)
			}
		})
	}
}
