//go:build !windows

package app

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// LockDir es el directorio de los archivos de bloqueo (inyectable en tests).
var LockDir = os.TempDir()

// Acquire toma un flock exclusivo no bloqueante sobre un archivo en LockDir.
// Si otra instancia lo tiene, devuelve ErrAlreadyRunning.
func Acquire(name string) (release func(), err error) {
	f, err := os.OpenFile(filepath.Join(LockDir, name+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrAlreadyRunning
		}
		return nil, err
	}
	return func() { f.Close() }, nil
}
