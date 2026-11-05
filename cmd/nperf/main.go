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
	"time"

	"nperf/internal/client"
	"nperf/internal/config"
	"nperf/internal/proto"
	"nperf/internal/report"
	"nperf/internal/server"
	"nperf/internal/version"
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
	asJSON := fs.Lookup("json") != nil && wantsJSON(args)
	usage := fs.Usage
	if asJSON { // con -json, stdout no debe llevar la ayuda que flag imprime ante un error
		fs.Usage = func() {}
	}
	err := fs.Parse(args)
	switch {
	case err == nil:
		return 0, false
	case errors.Is(err, flag.ErrHelp):
		if asJSON {
			usage()
		}
		return exitOK, true
	}
	msg := fmt.Errorf("argumentos inválidos (%v). Use -h para ver la ayuda", err)
	if asJSON {
		report.JSONError(stdout, msg)
	} else {
		fmt.Fprintln(stderr, "error:", msg)
	}
	return exitUsage, true
}

// wantsJSON dice si los argumentos piden -json (sin analizar los demás); se detiene ante "--".
func wantsJSON(args []string) bool {
	for _, a := range args {
		switch a {
		case "--":
			return false
		case "-json", "--json", "-json=true", "--json=true":
			return true
		}
	}
	return false
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

func runClient(ctx context.Context, args []string) int {
	fs := newFlagSet("client", "nperf client <host> [opciones]")
	port := fs.Int("port", defaultPort, fmt.Sprintf("puerto del servidor (por defecto: %d)", defaultPort))
	secs := fs.Int("duration", 10, fmt.Sprintf("duración de cada fase en segundos, de %d a %d (por defecto: 10)", minDuration, maxDuration))
	key := fs.String("key", "", keyHelp)
	protoName := fs.String("proto", "both", "protocolo: tcp, udp o both (both = RTT, TCP y UDP) (por defecto: both)")
	rateStr := fs.String("udp-rate", "", "tasa UDP (50M, 1G o bps); por defecto 100M, o 90 % del TCP con -proto both (por defecto: both)")
	streams := fs.Int("streams", proto.DefaultStreams, fmt.Sprintf("flujos TCP paralelos, de 1 a 64 (por defecto: %d)", proto.DefaultStreams))
	direction := fs.String("direction", "both", "sentido: fwd (cliente a servidor), rev (servidor a cliente) o both (por defecto: both)")
	jsonOut := fs.Bool("json", false, "imprime solo un objeto JSON (formato v1) por la salida estándar; los errores salen como {\"error\": \"...\"}")
	// Permite `client <host> -flags` además de `client -flags <host>`.
	var host string
	if len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
		host, args = args[0], args[1:]
	}
	if code, done := parse(fs, args); done {
		return code
	}
	fail := func(code int, err error) int {
		if *jsonOut {
			report.JSONError(stdout, err)
		} else {
			fmt.Fprintln(stderr, "error:", err)
		}
		return code
	}
	if host == "" && fs.NArg() > 0 {
		host = fs.Arg(0)
	}
	if host == "" {
		return fail(exitUsage, errors.New("falta el host. Uso: nperf client <host>"))
	}
	if err := validateDuration(*secs); err != nil {
		return fail(exitUsage, err)
	}
	if err := client.ValidateStreams(*streams); err != nil {
		return fail(exitUsage, err)
	}
	if *direction != "fwd" && *direction != "rev" && *direction != "both" {
		return fail(exitUsage, fmt.Errorf("sentido desconocido %q; use fwd, rev o both", *direction))
	}
	if *protoName != "tcp" && *protoName != "udp" && *protoName != "both" {
		return fail(exitUsage, fmt.Errorf("protocolo desconocido %q; use tcp, udp o both", *protoName))
	}
	var rate float64
	if *rateStr != "" {
		if *protoName == "tcp" {
			return fail(exitUsage, errors.New("-udp-rate solo aplica con -proto udp o both (por defecto: both)"))
		}
		var err error
		if rate, err = client.ParseRate(*rateStr); err != nil {
			return fail(exitUsage, err)
		}
	}
	k, err := resolveKey(fs, *key)
	if err != nil {
		return fail(exitUsage, err)
	}
	if k == "" {
		fmt.Fprintln(stderr, "aviso: conectando sin clave")
	}
	dur := time.Duration(*secs) * time.Second
	addr := net.JoinHostPort(host, strconv.Itoa(*port))
	opts := client.Options{Duration: dur, Proto: *protoName, Direction: *direction, UDPRate: rate, Key: k, Streams: *streams}
	if *jsonOut { // stdout solo lleva el JSON
		opts.OnQueue = func(pos int) { fmt.Fprintf(stderr, "en cola (posición %d)\n", pos) }
	} else {
		fmt.Fprintf(stdout, "%s hacia %s durante %d s por fase...\n", modeText(*protoName), addr, *secs)
		opts.OnQueue = func(pos int) { fmt.Fprintf(stdout, "en cola (posición %d)\n", pos) }
	}
	res, err := client.Run(ctx, addr, opts)
	if err != nil {
		return fail(exitError, err)
	}
	p := report.Params{Host: host, Port: *port, Proto: *protoName, Direction: *direction, Duration: dur,
		Streams: *streams, UDPRate: rate, Version: version.Current}
	if *jsonOut {
		if err := report.JSON(stdout, p, res); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return exitError
		}
		return exitOK
	}
	report.Text(stdout, p, res)
	return exitOK
}
