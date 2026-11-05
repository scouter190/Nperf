// Comando nperf: medidor de red en par (server / client).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"nperf/internal/client"
	"nperf/internal/config"
	"nperf/internal/meter"
	"nperf/internal/proto"
	"nperf/internal/server"
)

const (
	defaultPort = 5300
	minDuration = 3   // segundos
	maxDuration = 300 // segundos
)

// Códigos de salida.
const (
	exitOK    = 0 // medición o servidor terminados sin error
	exitError = 1 // error de medición, conexión o autenticación
	exitUsage = 2 // error de uso (argumentos o configuración inválidos)
)

// Variables sustituibles en las pruebas.
var (
	stdout    io.Writer = os.Stdout
	stderr    io.Writer = os.Stderr
	configDir           = os.UserConfigDir
)

func main() { os.Exit(run(os.Args[1:])) }

const generalHelp = `NPerf: medidor de red en par (RTT, TCP y UDP) entre dos equipos.

Uso:
  nperf server [opciones]          inicia el servidor de medición
  nperf client <host> [opciones]   mide contra un servidor
  nperf help                       muestra esta ayuda

Use "nperf server -h" o "nperf client -h" para ver las opciones de cada comando.

Clave compartida: opción -key o, si falta, el campo "key" de
config.json en el directorio de configuración del usuario, carpeta NPerf
(en Windows, %AppData%\NPerf\config.json). Formato: {"key": "clave"}.
La opción -key tiene prioridad sobre el archivo.

Códigos de salida:
  0  correcto
  1  error de medición, conexión o autenticación
  2  error de uso (argumentos o configuración inválidos)
`

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(stdout, generalHelp)
		return exitUsage
	}
	switch args[0] {
	case "-h", "-help", "--help", "help":
		fmt.Fprint(stdout, generalHelp)
		return exitOK
	case "server", "client":
	default:
		fmt.Fprintf(stderr, "comando desconocido: %s\n\n%s", args[0], generalHelp)
		return exitUsage
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if args[0] == "server" {
		return runServer(ctx, args[1:])
	}
	return runClient(ctx, args[1:])
}

// newFlagSet crea un FlagSet cuya ayuda y errores salen en español.
func newFlagSet(name, usage string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {
		fmt.Fprintf(stdout, "Uso: %s\n\nOpciones:\n", usage)
		fs.SetOutput(stdout)
		fs.VisitAll(func(f *flag.Flag) {
			fmt.Fprintf(stdout, "  -%s\n    \t%s\n", f.Name, f.Usage)
		})
		fs.SetOutput(io.Discard)
	}
	return fs
}

// parse analiza los argumentos; devuelve (código, true) si hay que terminar.
func parse(fs *flag.FlagSet, args []string) (int, bool) {
	err := fs.Parse(args)
	switch {
	case err == nil:
		return 0, false
	case errors.Is(err, flag.ErrHelp):
		return exitOK, true
	}
	fmt.Fprintf(stderr, "error: argumentos inválidos (%v). Use -h para ver la ayuda\n", err)
	return exitUsage, true
}

const keyHelp = "clave compartida; si falta se lee config.json (campo \"key\") del directorio de configuración del usuario; vacía = sin clave"

// resolveKey aplica la prioridad: -key (si se indicó) y luego config.json.
func resolveKey(fs *flag.FlagSet, flagKey string) (string, error) {
	given := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "key" {
			given = true
		}
	})
	if given {
		return flagKey, nil
	}
	dir, err := configDir()
	if err != nil { // sin directorio de configuración: no hay archivo que leer
		return "", nil
	}
	c, err := config.Load(dir)
	if err != nil {
		return "", err
	}
	return c.Key, nil
}

func runServer(ctx context.Context, args []string) int {
	fs := newFlagSet("server", "nperf server [opciones]")
	port := fs.Int("port", defaultPort, fmt.Sprintf("puerto de escucha, TCP y UDP (por defecto: %d)", defaultPort))
	key := fs.String("key", "", keyHelp)
	if code, done := parse(fs, args); done {
		return code
	}
	k, err := resolveKey(fs, *key)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return exitUsage
	}
	if k == "" {
		fmt.Fprintln(stderr, "aviso: sin clave, cualquiera puede usar este servidor")
	}
	lc := net.ListenConfig{KeepAlive: 15 * time.Second} // detecta clientes en cola con la conexión caída
	ln, err := lc.Listen(ctx, "tcp", ":"+strconv.Itoa(*port))
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return exitError
	}
	fmt.Fprintf(stdout, "Servidor escuchando en %s\n", ln.Addr())
	if err := server.New(server.WithKey(k), server.WithLog(func(m string) { fmt.Fprintln(stderr, m) })).Serve(ctx, ln); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return exitError
	}
	return exitOK
}

// validateDuration comprueba el rango permitido de -duration (segundos).
func validateDuration(secs int) error {
	if secs < minDuration || secs > maxDuration {
		return fmt.Errorf("-duration debe estar entre %d y %d segundos (se pidió %d)", minDuration, maxDuration, secs)
	}
	return nil
}

// modeText es el texto de la línea "Midiendo ..." según el protocolo.
func modeText(protoName string) string {
	switch protoName {
	case "tcp":
		return "Midiendo TCP"
	case "udp":
		return "Midiendo UDP"
	}
	return "Medición completa (RTT, TCP y UDP)"
}

// rateNote devuelve la nota de "tasa no alcanzada" si lo enviado queda por debajo del 90 %
// de la objetivo (bps); "" si no aplica.
func rateNote(dir string, d client.Dir, targetBps float64) string {
	if !d.Measured || d.UDP == nil || targetBps <= 0 {
		return ""
	}
	target := targetBps / 1e6
	if d.UDP.SentMbps >= 0.9*target {
		return ""
	}
	return fmt.Sprintf("UDP %s: no se alcanzó la tasa objetivo (enviado %.1f de %.1f Mbps)", dir, d.UDP.SentMbps, target)
}

func runClient(ctx context.Context, args []string) int {
	fs := newFlagSet("client", "nperf client <host> [opciones]")
	port := fs.Int("port", defaultPort, fmt.Sprintf("puerto del servidor (por defecto: %d)", defaultPort))
	secs := fs.Int("duration", 10, fmt.Sprintf("duración de cada fase en segundos, de %d a %d (por defecto: 10)", minDuration, maxDuration))
	key := fs.String("key", "", keyHelp)
	protoName := fs.String("proto", "both", "protocolo: tcp, udp o both (both = RTT, TCP y UDP) (por defecto: both)")
	rateStr := fs.String("udp-rate", "", "tasa UDP (50M, 1G o bps); por defecto 100M, o 90 % del TCP con -proto both (por defecto: both)")
	streams := fs.Int("streams", proto.DefaultStreams, fmt.Sprintf("flujos TCP paralelos, de 1 a 64 (por defecto: %d)", proto.DefaultStreams))
	direction := fs.String("direction", "both", "sentido: fwd (cliente a servidor), rev (servidor a cliente) o both (por defecto: both)")
	// Permite `client <host> -flags` además de `client -flags <host>`.
	var host string
	if len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
		host, args = args[0], args[1:]
	}
	if code, done := parse(fs, args); done {
		return code
	}
	if host == "" && fs.NArg() > 0 {
		host = fs.Arg(0)
	}
	if host == "" {
		fmt.Fprintln(stderr, "error: falta el host. Uso: nperf client <host>")
		return exitUsage
	}
	if err := validateDuration(*secs); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return exitUsage
	}
	if err := client.ValidateStreams(*streams); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return exitUsage
	}
	if *direction != "fwd" && *direction != "rev" && *direction != "both" {
		fmt.Fprintf(stderr, "error: sentido desconocido %q; use fwd, rev o both\n", *direction)
		return exitUsage
	}
	if *protoName != "tcp" && *protoName != "udp" && *protoName != "both" {
		fmt.Fprintf(stderr, "error: protocolo desconocido %q; use tcp, udp o both\n", *protoName)
		return exitUsage
	}
	var rate float64
	if *rateStr != "" {
		if *protoName == "tcp" {
			fmt.Fprintln(stderr, "error: -udp-rate solo aplica con -proto udp o both (por defecto: both)")
			return exitUsage
		}
		var err error
		if rate, err = client.ParseRate(*rateStr); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return exitUsage
		}
	}
	k, err := resolveKey(fs, *key)
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return exitUsage
	}
	if k == "" {
		fmt.Fprintln(stderr, "aviso: conectando sin clave")
	}
	dur := time.Duration(*secs) * time.Second
	addr := net.JoinHostPort(host, strconv.Itoa(*port))
	fmt.Fprintf(stdout, "%s hacia %s durante %d s por fase...\n", modeText(*protoName), addr, *secs)
	res, err := client.Run(ctx, addr, client.Options{Duration: dur, Proto: *protoName, Direction: *direction, UDPRate: rate, Key: k, Streams: *streams,
		OnQueue: func(pos int) { fmt.Fprintf(stdout, "en cola (posición %d)\n", pos) }})
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return exitError
	}
	var notes []string
	if *protoName == "both" {
		if r := res.RTT; r != nil {
			fmt.Fprintf(stdout, "RTT: mín %s, media %s, máx %s (%d pings)\n",
				fmtRTT(r.Min), fmtRTT(r.Avg), fmtRTT(r.Max), r.Samples)
		}
		printDir("tcp", "fwd", "cliente -> servidor", res.Fwd)
		printDir("tcp", "rev", "servidor -> cliente", res.Rev)
		printDir("udp", "fwd", "cliente -> servidor", res.UDPFwd)
		printDir("udp", "rev", "servidor -> cliente", res.UDPRev)
		notes = append(notes, res.Notes...)
		notes = append(notes, rateNote("fwd", res.UDPFwd, res.UDPRateFwd), rateNote("rev", res.UDPRev, res.UDPRateRev))
	} else {
		printDir(*protoName, "fwd", "cliente -> servidor", res.Fwd)
		printDir(*protoName, "rev", "servidor -> cliente", res.Rev)
		if *protoName == "udp" {
			target := rate
			if target == 0 {
				target = client.DefaultUDPRate
			}
			notes = append(notes, rateNote("fwd", res.Fwd, target), rateNote("rev", res.Rev, target))
		}
	}
	for _, n := range notes {
		if n != "" {
			fmt.Fprintln(stdout, "nota:", n)
		}
	}
	return exitOK
}

// fmtRTT da formato a un RTT en ms; por debajo de 1 µs (incluido 0, por la resolución del
// reloj) muestra "<1 µs".
func fmtRTT(d time.Duration) string {
	if d < time.Microsecond {
		return "<1 µs"
	}
	return fmt.Sprintf("%.2f ms", float64(d)/float64(time.Millisecond))
}

func printDir(proto, dir, label string, d client.Dir) {
	name := strings.ToUpper(proto)
	if !d.Measured {
		fmt.Fprintf(stdout, "%s %s (%s): no medido\n", name, dir, label)
		return
	}
	if u := d.UDP; u != nil {
		fmt.Fprintf(stdout, "%s %s (%s): enviado %.1f Mbps, recibido %.1f Mbps, jitter %.3f ms, pérdida %.2f %% (%d de %d datagramas)\n",
			name, dir, label, u.SentMbps, d.Mbps, u.JitterMS, u.LossPct, u.Lost, u.Expected)
		return
	}
	fmt.Fprintf(stdout, "%s %s (%s): %.1f Mbps (%d bytes en %.1f s), retransmisiones: %s\n",
		name, dir, label, d.Mbps, d.Received.Bytes, d.Received.Elapsed.Seconds(), fmtRetrans(d.Sent))
}

// fmtRetrans da formato a las retransmisiones del emisor o "no disponible".
func fmtRetrans(r meter.Result) string {
	if !r.RetransOK {
		return "no disponible"
	}
	if r.RetransEstimated {
		return "~" + strconv.FormatUint(r.Retrans, 10) + " (estimado)"
	}
	return strconv.FormatUint(r.Retrans, 10)
}
