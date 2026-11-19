package ndt7

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

// DailyLimit es el máximo de pruebas M-Lab al día por equipo/perfil (AUP de M-Lab).
const DailyLimit = 40

const (
	quotaFile    = "mlab-diario.json"
	lockFile     = "mlab-diario.lock"
	lockWait     = 2 * time.Second
	lockStale    = 30 * time.Second
	lockInterval = 5 * time.Millisecond
)

// ErrQuotaExceeded indica que ya se usaron las pruebas del día.
var ErrQuotaExceeded = errors.New("límite diario de pruebas M-Lab alcanzado")

// Quota lleva el contador diario persistente. Dir es el directorio de NPerf
// y Clock (opcional, por defecto time.Now) se usa para la edad del bloqueo.
type Quota struct {
	Dir   string
	Clock func() time.Time
}

type quotaState struct {
	Day   string `json:"day"`
	Count int    `json:"count"`
}

func (q *Quota) now() time.Time {
	if q.Clock != nil {
		return q.Clock()
	}
	return time.Now()
}

// Reserve cuenta una prueba iniciada en el día local de now y devuelve cuántas
// van hoy. Si ya había 40 devuelve ErrQuotaExceeded sin incrementar. Un error
// de E/S se devuelve tal cual: no se debe medir.
func (q *Quota) Reserve(now time.Time) (int, error) {
	if err := os.MkdirAll(q.Dir, 0o700); err != nil {
		return 0, fmt.Errorf("no se pudo comprobar el límite diario: %w", err)
	}
	unlock, err := q.lock()
	if err != nil {
		return 0, fmt.Errorf("no se pudo comprobar el límite diario: %w", err)
	}
	defer unlock()

	day := now.Format("2006-01-02")
	path := filepath.Join(q.Dir, quotaFile)
	st := quotaState{Day: day}
	var data []byte
	err = retry(func() (err error) { data, err = os.ReadFile(path); return })
	switch {
	case err == nil:
		var read quotaState
		if json.Unmarshal(data, &read) == nil && read.Day == day && read.Count > 0 {
			st.Count = read.Count
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		return 0, fmt.Errorf("no se pudo comprobar el límite diario: %w", err)
	}
	if st.Count >= DailyLimit {
		return st.Count, ErrQuotaExceeded
	}
	st.Count++
	if err := writeAtomic(path, st); err != nil {
		return 0, fmt.Errorf("no se pudo comprobar el límite diario: %w", err)
	}
	return st.Count, nil
}

func writeAtomic(path string, st quotaState) error {
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "mlab-diario-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, werr := tmp.Write(b)
	if werr == nil {
		werr = tmp.Sync()
	}
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		os.Remove(name)
		if werr != nil {
			return werr
		}
		return cerr
	}
	if err := retry(func() error { return os.Rename(name, path) }); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// retry repite fn mientras falle con un error transitorio (Windows), hasta
// lockWait. Los errores reales se devuelven sin tocar.
func retry(fn func() error) error {
	deadline := time.Now().Add(lockWait)
	for {
		err := fn()
		if err == nil || !isTransient(err) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(lockInterval + time.Duration(rand.Intn(5))*time.Millisecond)
	}
}

// releaseLock borra el bloqueo solo si aún contiene nuestro token, para no
// quitar el de otro proceso que rompió el nuestro por huérfano. Queda una
// ventana residual mínima entre leer y borrar (aceptable: límite de cortesía).
func releaseLock(path, token string) {
	var b []byte
	if retry(func() (err error) { b, err = os.ReadFile(path); return }) == nil && string(b) == token {
		retry(func() error { return os.Remove(path) })
	}
}

// lock toma el bloqueo por archivo (O_CREATE|O_EXCL), reintentando hasta 2 s y
// rompiendo uno huérfano de más de 30 s.
func (q *Quota) lock() (func(), error) {
	path := filepath.Join(q.Dir, lockFile)
	deadline := time.Now().Add(lockWait)
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			token := fmt.Sprintf("%d-%d", os.Getpid(), rand.Int63())
			_, werr := f.WriteString(token)
			f.Close()
			if werr != nil {
				os.Remove(path)
				return nil, werr
			}
			return func() { releaseLock(path, token) }, nil
		}
		// En Windows un bloqueo abierto o pendiente de borrar da acceso
		// denegado o violación de uso compartido: se trata como ocupado.
		if !errors.Is(err, os.ErrExist) && !isTransient(err) {
			return nil, err
		}
		if fi, serr := os.Stat(path); serr == nil && q.now().Sub(fi.ModTime()) > lockStale {
			os.Remove(path)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("bloqueo ocupado: %w", err)
		}
		time.Sleep(lockInterval + time.Duration(rand.Intn(5))*time.Millisecond)
	}
}
