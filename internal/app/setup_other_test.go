//go:build !windows

package app

import "testing"

func setup(t *testing.T) {
	old := LockDir
	LockDir = t.TempDir()
	t.Cleanup(func() { LockDir = old })
}
