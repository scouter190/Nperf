// Package diag evalúa el semáforo de cuello de botella de una prueba TCP.
// Es lógica pura: no depende de netinfo ni de proto.
package diag

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Kind es el tipo de conexión de un extremo.
type Kind int

const (
	KindUnknown Kind = iota
	KindWired
	KindWiFi
	KindOther
)

// Color es el resultado del semáforo.
type Color string

const (
	None  Color = ""
	Green Color = "green"
	Amber Color = "amber"
	Red   Color = "red"
)

// End describe el enlace de un extremo. Known=false o velocidad 0 = sin datos.
// Signal es el % de señal WiFi (< 0 = desconocida).
type End struct {
	Alias  string
	Known  bool
	Kind   Kind
	TxBps  uint64
	RxBps  uint64
	Signal int
}

// Result es el diagnóstico de un sentido.
type Result struct {
	Color       Color
	Percent     int     // 0-100 (se limita a 100)
	Nominal     uint64  // enlace nominal usado, bps
	Ratio       float64 // throughput/nominal en %, sin redondear ni limitar
	Partial     bool    // solo se conocía un extremo
	Over        bool    // throughput > nominal
	Both        bool    // empate: limitan ambos extremos
	Limiter     string  // alias del extremo limitante ("" si empate o sin datos)
	LimiterKind Kind
	Text        string
}

// Evaluate calcula el semáforo de un sentido. throughputBps < 0 = no medido
// (sin semáforo). El nominal es mín(tx del emisor, rx del receptor).
func Evaluate(throughputBps float64, sender, receiver End) Result {
	if throughputBps < 0 || math.IsNaN(throughputBps) {
		return Result{Text: "Sentido no medido: sin diagnóstico."}
	}
	sOK := sender.Known && sender.TxBps > 0
	rOK := receiver.Known && receiver.RxBps > 0
	if !sOK && !rOK {
		return Result{Text: "Sin datos de enlace de ninguno de los extremos: sin diagnóstico."}
	}
	var r Result
	var lim End
	var limSpeed uint64
	switch {
	case sOK && !rOK:
		lim, limSpeed, r.Partial = sender, sender.TxBps, true
	case rOK && !sOK:
		lim, limSpeed, r.Partial = receiver, receiver.RxBps, true
	case sender.TxBps < receiver.RxBps:
		lim, limSpeed = sender, sender.TxBps
	case receiver.RxBps < sender.TxBps:
		lim, limSpeed = receiver, receiver.RxBps
	default:
		r.Both = true
		limSpeed = sender.TxBps
		lim = sender
		if receiver.Kind == KindWiFi {
			lim = receiver // umbrales WiFi si alguno lo es
		}
	}
	r.Nominal = limSpeed
	r.LimiterKind = lim.Kind
	if !r.Both {
		r.Limiter = lim.Alias
	}
	r.Ratio = throughputBps / float64(limSpeed) * 100
	pct := r.Ratio
	if pct > 100 {
		pct, r.Over = 100, true
	}
	r.Percent = int(math.Round(pct))

	green, amber := 80.0, 50.0
	if r.Both && (sender.Kind == KindWiFi || receiver.Kind == KindWiFi) || lim.Kind == KindWiFi {
		green, amber = 50.0, 30.0
	}
	switch {
	case r.Ratio > green:
		r.Color = Green
	case r.Ratio >= amber:
		r.Color = Amber
	default:
		r.Color = Red
	}
	r.Text = text(r, lim)
	return r
}

func text(r Result, lim End) string {
	var b strings.Builder
	link := "enlace " + FormatBps(r.Nominal)
	if lim.Kind == KindWiFi && lim.Signal >= 0 && !r.Both {
		link += ", señal " + strconv.Itoa(lim.Signal) + " %"
	}
	if r.Both {
		fmt.Fprintf(&b, "Limitado por ambos extremos (%s)", link)
	} else {
		fmt.Fprintf(&b, "Limitado por %s de %s (%s)", kindName(lim.Kind), lim.Alias, link)
	}
	b.WriteString(".")
	if r.Color == Amber || r.Color == Red {
		b.WriteString(" Por debajo del enlace más lento: posible cuello en la red intermedia, la señal WiFi o el equipo.")
	}
	if r.Over {
		b.WriteString(" Medido por encima del enlace anunciado: se muestra 100 %.")
	}
	if r.Partial {
		b.WriteString(" Diagnóstico parcial: faltan datos del otro extremo.")
	}
	return b.String()
}

func kindName(k Kind) string {
	switch k {
	case KindWiFi:
		return "el WiFi"
	case KindWired:
		return "el cable"
	}
	return "la conexión"
}

// FormatBps da un texto corto en español: 100 Mbps, 1 Gbps, 2.5 Gbps.
func FormatBps(bps uint64) string {
	switch {
	case bps >= 1_000_000_000:
		return trim(float64(bps)/1e9) + " Gbps"
	case bps >= 1_000_000:
		return trim(float64(bps)/1e6) + " Mbps"
	}
	return trim(float64(bps)/1e3) + " Kbps"
}

func trim(f float64) string {
	return strconv.FormatFloat(math.Round(f*100)/100, 'f', -1, 64)
}
