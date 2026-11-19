package history

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Límites de retención por defecto.
const (
	DefaultMaxAge   = 90 * 24 * time.Hour
	DefaultMaxBytes = 50 << 20
)

// ErrDisabled indica que el historial está desactivado y no se guardó nada.
var ErrDisabled = errors.New("historial desactivado")

// saveMu serializa guardado y retención dentro del proceso.
const (
	tmpPrefix = ".tmp-"
	tmpMaxAge = time.Hour
)

var saveMu sync.Mutex

// DefaultDir devuelve <localAppData>\NPerf\historial ("" si localAppData está vacío).
func DefaultDir(localAppData string) string {
	if localAppData == "" {
		return ""
	}
	return filepath.Join(localAppData, "NPerf", "historial")
}

// Store guarda registros como archivos JSON en Dir.
type Store struct {
	Dir      string
	Clock    func() time.Time // nil: time.Now
	Enabled  bool
	MaxAge   time.Duration // 0: DefaultMaxAge
	MaxBytes int64         // 0: DefaultMaxBytes

	rand     io.Reader                     // nil: crypto/rand
	linkFn   func(oldp, newp string) error // nil: os.Link
	renameFn func(oldp, newp string) error // nil: os.Rename
}

// Open crea el directorio (si está activado) y aplica la retención.
func Open(dir string, clock func() time.Time, enabled bool) (*Store, error) {
	s := &Store{Dir: dir, Clock: clock, Enabled: enabled}
	if !enabled {
		return s, nil
	}
	if dir == "" {
		return nil, errors.New("historial: directorio vacío")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("historial: %w", err)
	}
	saveMu.Lock()
	defer saveMu.Unlock()
	s.prune()
	return s, nil
}

func (s *Store) now() time.Time {
	if s.Clock != nil {
		return s.Clock()
	}
	return time.Now()
}

// Save escribe r de forma atómica (temporal + enlace sin pisar) y aplica la retención.
// Completa ID, Time y FormatVersion si faltan. Devuelve el id.
func (s *Store) Save(r *Record) (string, error) {
	if !s.Enabled {
		return "", ErrDisabled
	}
	if s.Dir == "" {
		return "", errors.New("historial: directorio vacío")
	}
	now := s.now()
	if r.FormatVersion == 0 {
		r.FormatVersion = FormatVersion
	}
	if r.Time == "" {
		r.Time = now.UTC().Format(time.RFC3339)
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return "", fmt.Errorf("historial: %w", err)
	}
	saveMu.Lock()
	defer saveMu.Unlock()

	rd := s.rand
	if rd == nil {
		rd = rand.Reader
	}
	explicit := r.ID != ""
	for i := 0; i < 8; i++ {
		if !explicit {
			id, err := NewID(now, rd)
			if err != nil {
				return "", err
			}
			r.ID = id
		}
		data, err := r.Encode()
		if err != nil {
			return "", err
		}
		tmpName, err := s.writeTemp(data)
		if err != nil {
			return "", err
		}
		// Publicación que no pisa: falla si el destino ya existe.
		err = s.link(tmpName, s.path(r.ID))
		if err != nil && !errors.Is(err, os.ErrExist) {
			// Sin enlaces duros (FAT/exFAT, carpetas redirigidas): Lstat + rename.
			if _, e := os.Lstat(s.path(r.ID)); e == nil {
				err = os.ErrExist
			} else if e := s.mv(tmpName, s.path(r.ID)); e == nil {
				err = nil
			}
		}
		os.Remove(tmpName)
		if err == nil {
			s.prune()
			return r.ID, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("historial: %w", err)
		}
		if explicit {
			return "", fmt.Errorf("historial: el id %s ya existe", r.ID)
		}
	}
	return "", errors.New("historial: no se pudo generar un id único")
}

func (s *Store) mv(oldp, newp string) error {
	if s.renameFn != nil {
		return s.renameFn(oldp, newp)
	}
	return os.Rename(oldp, newp)
}

func (s *Store) link(oldp, newp string) error {
	if s.linkFn != nil {
		return s.linkFn(oldp, newp)
	}
	return os.Link(oldp, newp)
}

func (s *Store) writeTemp(data []byte) (string, error) {
	tmp, err := os.CreateTemp(s.Dir, tmpPrefix+"*")
	if err != nil {
		return "", fmt.Errorf("historial: %w", err)
	}
	name := tmp.Name()
	_, werr := tmp.Write(data)
	if werr == nil {
		werr = tmp.Sync()
	}
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		os.Remove(name)
		return "", fmt.Errorf("historial: %w", werr)
	}
	return name, nil
}

// Prune aplica la retención: > MaxAge y luego > MaxBytes (más antiguos primero).
func (s *Store) Prune() {
	if !s.Enabled || s.Dir == "" {
		return
	}
	saveMu.Lock()
	defer saveMu.Unlock()
	s.prune()
}

func (s *Store) path(id string) string { return filepath.Join(s.Dir, id+".json") }

type entry struct {
	id   string
	size int64
}

func (s *Store) prune() {
	des, err := os.ReadDir(s.Dir)
	if err != nil {
		return
	}
	var es []entry
	now0 := s.now()
	for _, d := range des {
		name := d.Name()
		if strings.HasPrefix(name, tmpPrefix) && d.Type().IsRegular() {
			if fi, err := d.Info(); err == nil && now0.Sub(fi.ModTime()) > tmpMaxAge {
				os.Remove(filepath.Join(s.Dir, name))
			}
			continue
		}
		if !strings.HasSuffix(name, ".json") || !ValidID(strings.TrimSuffix(name, ".json")) || !d.Type().IsRegular() {
			continue
		}
		fi, err := d.Info()
		if err != nil {
			continue
		}
		es = append(es, entry{strings.TrimSuffix(name, ".json"), fi.Size()})
	}
	sort.Slice(es, func(i, j int) bool { return es[i].id < es[j].id })
	maxAge, maxBytes := s.MaxAge, s.MaxBytes
	if maxAge == 0 {
		maxAge = DefaultMaxAge
	}
	if maxBytes == 0 {
		maxBytes = DefaultMaxBytes
	}
	now := s.now()
	var kept []entry
	for _, e := range es {
		t, err := time.Parse("20060102-150405", e.id[:15])
		if err == nil && now.Sub(t) > maxAge && os.Remove(s.path(e.id)) == nil {
			continue
		}
		kept = append(kept, e)
	}
	var total int64
	for _, e := range kept {
		total += e.size
	}
	for _, e := range kept {
		if total <= maxBytes {
			break
		}
		if os.Remove(s.path(e.id)) == nil {
			total -= e.size
		}
	}
}
