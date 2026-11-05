// Comando nperf: medidor de red en par (server / client).
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"nperf/internal/client"
	"nperf/internal/server"
)

const defaultPort = 5300

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "uso: nperf server | nperf client <host>")
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	switch args[0] {
	case "server":
		return runServer(ctx, args[1:])
	case "client":
		return runClient(ctx, args[1:])
	}
	fmt.Fprintf(os.Stderr, "comando desconocido: %s\n", args[0])
	return 2
}

func runServer(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("server", flag.ContinueOnError)
	port := fs.Int("port", defaultPort, "puerto de escucha")
	key := fs.String("key", "", "shared key (empty = no key; both sides must then be empty)")
	if fs.Parse(args) != nil {
		return 2
	}
	if *key == "" {
		fmt.Fprintln(os.Stderr, "aviso: sin clave, cualquiera puede usar este servidor")
	}
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(*port))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	fmt.Printf("Servidor escuchando en %s\n", ln.Addr())
	if err := server.New(server.WithKey(*key), server.WithLog(func(m string) { fmt.Fprintln(os.Stderr, m) })).Serve(ctx, ln); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

func runClient(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("client", flag.ContinueOnError)
	port := fs.Int("port", defaultPort, "puerto del servidor")
	secs := fs.Int("duration", 10, "duración de la prueba en segundos")
	key := fs.String("key", "", "shared key (empty = no key; both sides must then be empty)")
	protoName := fs.String("proto", "tcp", "protocolo: tcp o udp")
	rateStr := fs.String("udp-rate", "", "tasa UDP (50M, 1G o bps); por defecto 100M")
	direction := fs.String("direction", "both", "sentido: fwd, rev o both")
	// Permite `client <host> -flags` además de `client -flags <host>`.
	var host string
	if len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
		host, args = args[0], args[1:]
	}
	if fs.Parse(args) != nil {
		return 2
	}
	if host == "" && fs.NArg() > 0 {
		host = fs.Arg(0)
	}
	if host == "" {
		fmt.Fprintln(os.Stderr, "error: falta el host. Uso: nperf client <host>")
		return 2
	}
	if *direction != "fwd" && *direction != "rev" && *direction != "both" {
		fmt.Fprintf(os.Stderr, "error: sentido desconocido %q; use fwd, rev o both\n", *direction)
		return 2
	}
	if *protoName == "both" {
		fmt.Fprintln(os.Stderr, "error: -proto both aún no está disponible; use tcp o udp")
		return 2
	}
	if *protoName != "tcp" && *protoName != "udp" {
		fmt.Fprintf(os.Stderr, "error: protocolo desconocido %q; use tcp o udp\n", *protoName)
		return 2
	}
	var rate float64
	if *rateStr != "" {
		if *protoName != "udp" {
			fmt.Fprintln(os.Stderr, "error: -udp-rate solo aplica con -proto udp")
			return 2
		}
		var err error
		if rate, err = client.ParseRate(*rateStr); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 2
		}
	}
	if *key == "" {
		fmt.Fprintln(os.Stderr, "aviso: conectando sin clave")
	}
	dur := time.Duration(*secs) * time.Second
	addr := net.JoinHostPort(host, strconv.Itoa(*port))
	fmt.Printf("Midiendo %s hacia %s durante %d s...\n", strings.ToUpper(*protoName), addr, *secs)
	res, err := client.Run(ctx, addr, client.Options{Duration: dur, Proto: *protoName, Direction: *direction, UDPRate: rate, Key: *key})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	printDir(*protoName, "fwd", "cliente -> servidor", res.Fwd)
	printDir(*protoName, "rev", "servidor -> cliente", res.Rev)
	return 0
}

func printDir(proto, dir, label string, d client.Dir) {
	name := strings.ToUpper(proto)
	if !d.Measured {
		fmt.Printf("%s %s (%s): no medido\n", name, dir, label)
		return
	}
	if u := d.UDP; u != nil {
		fmt.Printf("%s %s (%s): enviado %.1f Mbps, recibido %.1f Mbps, jitter %.3f ms, pérdida %.2f %% (%d de %d datagramas)\n",
			name, dir, label, u.SentMbps, d.Mbps, u.JitterMS, u.LossPct, u.Lost, u.Expected)
		return
	}
	fmt.Printf("%s %s (%s): %.1f Mbps (%d bytes en %.1f s)\n",
		name, dir, label, d.Mbps, d.Received.Bytes, d.Received.Elapsed.Seconds())
}
