package client

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestGreetNoAck(t *testing.T) {
	greetTries, greetWait = 3, 20*time.Millisecond
	defer func() { greetTries, greetWait = 10, 300*time.Millisecond }()
	sink, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer sink.Close()
	uc, err := net.Dial("udp", sink.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer uc.Close()
	err = greetUDP(context.Background(), uc, "x")
	if err == nil || !strings.Contains(err.Error(), "no respondió") {
		t.Fatalf("error = %v", err)
	}
}
