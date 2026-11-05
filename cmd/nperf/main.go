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
	if fs.Parse(args) != nil {
		return 2
	}
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(*port))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	fmt.Printf("Servidor escuchando en %s\n", ln.Addr())
	if err := server.New().Serve(ctx, ln); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

func runClient(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("client", flag.ContinueOnError)
	port := fs.Int("port", defaultPort, "puerto del servidor")
	secs := fs.Int("duration", 10, "duración de la prueba en segundos")
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
	dur := time.Duration(*secs) * time.Second
	addr := net.JoinHostPort(host, strconv.Itoa(*port))
	fmt.Printf("Midiendo TCP hacia %s durante %d s...\n", addr, *secs)
	res, err := client.Run(ctx, addr, client.Options{Duration: dur})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	fmt.Printf("TCP fwd (cliente -> servidor): %.1f Mbps (%d bytes en %.1f s)\n",
		res.Mbps, res.Received.Bytes, res.Received.Elapsed.Seconds())
	return 0
}
