package client_test

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"nperf/internal/client"
)

// Un dial que nunca responde falla con el mensaje de tiempo agotado, sin red real.
func TestDialTimeout(t *testing.T) {
	o := client.Options{Duration: 200 * time.Millisecond, DialTimeout: 100 * time.Millisecond,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}}
	start := time.Now()
	_, err := client.Run(context.Background(), "10.255.255.1:5300", o)
	if err == nil || err.Error() != "no se pudo conectar a 10.255.255.1:5300 (tiempo agotado)" {
		t.Fatalf("err = %v", err)
	}
	if time.Since(start) > 2*time.Second || strings.Contains(err.Error(), "context") {
		t.Fatalf("tardó %v", time.Since(start))
	}
}
