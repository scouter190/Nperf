package client_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"nperf/internal/client"
)

// El server rechaza una tasa UDP rev por encima de 10 Gbps con un error claro.
func TestServerRejectsUDPRevRateAboveCap(t *testing.T) {
	addr, cancel, done := start(t)
	defer func() { cancel(); <-done }()
	_, err := client.Run(context.Background(), addr, client.Options{
		Duration: time.Second, Proto: "udp", Direction: "rev", UDPRate: 11e9})
	if err == nil || !strings.Contains(err.Error(), "demasiado alta") {
		t.Fatalf("error = %v, quería 'tasa UDP demasiado alta'", err)
	}
}
