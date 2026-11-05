package app

import (
	"errors"
	"fmt"
	"os"
	"testing"
)

func testName(t *testing.T, s string) string {
	return fmt.Sprintf("NPerfTest-%d-%s-%s", os.Getpid(), t.Name(), s)
}

func TestSegundaAdquisicionFalla(t *testing.T) {
	setup(t)
	n := testName(t, "a")
	rel, err := Acquire(n)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()
	if _, err := Acquire(n); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("esperaba ErrAlreadyRunning, got %v", err)
	}
}

func TestReadquirirTrasRelease(t *testing.T) {
	setup(t)
	n := testName(t, "a")
	rel, err := Acquire(n)
	if err != nil {
		t.Fatal(err)
	}
	rel()
	rel2, err := Acquire(n)
	if err != nil {
		t.Fatalf("tras release: %v", err)
	}
	rel2()
}

func TestNombresDistintos(t *testing.T) {
	setup(t)
	r1, err := Acquire(testName(t, "a"))
	if err != nil {
		t.Fatal(err)
	}
	defer r1()
	r2, err := Acquire(testName(t, "b"))
	if err != nil {
		t.Fatalf("nombre distinto: %v", err)
	}
	r2()
}
