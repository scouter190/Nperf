package history

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// List, Get, Delete y DeleteAll delegan en las funciones *Dir con s.Dir.
func (s *Store) List(f Filter) (ListResult, error) { return ListDir(s.Dir, f) }

// Get lee un registro por id.
func (s *Store) Get(id string) (*Record, error) { return GetDir(s.Dir, id) }

// Delete borra los ids indicados.
func (s *Store) Delete(ids []string) (int, error) { return DeleteDir(s.Dir, ids) }

// DeleteAll borra todos los registros.
func (s *Store) DeleteAll() (int, error) { return DeleteAllDir(s.Dir) }

// Filter acota List. Los campos vacíos no filtran.
type Filter struct {
	Host  string    // alias o IP (sin distinguir mayúsculas) de origen o destino
	Kind  string    // tipo exacto
	Since time.Time // inclusive
	Until time.Time // inclusive
}

// ListResult es el resultado de ListDir.
type ListResult struct {
	Records []*Record // más recientes primero
	Skipped int       // archivos *.json ilegibles, corruptos, grandes o inválidos
}

const ext = ".json"

// idFromName devuelve el id si name es "<id válido>.json".
func idFromName(name string) (string, bool) {
	id, ok := strings.CutSuffix(name, ext)
	if !ok || !ValidID(id) {
		return "", false
	}
	return id, true
}

func readRecord(dir, id string) (*Record, error) {
	if !ValidID(id) {
		return nil, errors.New("id no válido")
	}
	p := filepath.Join(dir, id+ext)
	fi, err := os.Lstat(p)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() || fi.Size() > MaxRecordBytes {
		return nil, errors.New("archivo no válido")
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxRecordBytes+1))
	if err != nil {
		return nil, err
	}
	r, err := Decode(data)
	if err != nil {
		return nil, err
	}
	if r.ID != id {
		return nil, errors.New("el id no coincide con el nombre del archivo")
	}
	return r, nil
}

func (f Filter) match(r *Record) bool {
	if f.Kind != "" && r.Kind != f.Kind {
		return false
	}
	if f.Host != "" {
		ok := false
		for _, e := range []*Endpoint{r.Source, r.Dest} {
			if e != nil && (strings.EqualFold(e.IP, f.Host) || strings.EqualFold(e.Alias, f.Host)) {
				ok = true
			}
		}
		if !ok {
			return false
		}
	}
	if !f.Since.IsZero() || !f.Until.IsZero() {
		t, err := time.Parse(time.RFC3339, r.Time)
		if err != nil {
			return false
		}
		if !f.Since.IsZero() && t.Before(f.Since) {
			return false
		}
		if !f.Until.IsZero() && t.After(f.Until) {
			return false
		}
	}
	return true
}

// ListDir lee los registros de dir, filtrados y ordenados por fecha
// descendente. Un directorio inexistente da lista vacía.
func ListDir(dir string, f Filter) (ListResult, error) {
	var res ListResult
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return res, nil
	}
	if err != nil {
		return res, err
	}
	times := map[*Record]time.Time{}
	for _, e := range entries {
		id, ok := idFromName(e.Name())
		if !ok {
			continue // archivo ajeno
		}
		r, err := readRecord(dir, id)
		if err != nil {
			res.Skipped++
			continue
		}
		if !f.match(r) {
			continue
		}
		t, _ := time.Parse(time.RFC3339, r.Time)
		times[r] = t
		res.Records = append(res.Records, r)
	}
	sort.Slice(res.Records, func(i, j int) bool {
		a, b := res.Records[i], res.Records[j]
		if ta, tb := times[a], times[b]; !ta.Equal(tb) {
			return ta.After(tb)
		}
		return a.ID > b.ID
	})
	return res, nil
}

// GetDir lee un registro por id (validado).
func GetDir(dir, id string) (*Record, error) { return readRecord(dir, id) }

// DeleteDir borra los ids indicados. Valida todos antes de borrar; un id
// inexistente no es error. Devuelve cuántos archivos se borraron.
func DeleteDir(dir string, ids []string) (int, error) {
	for _, id := range ids {
		if !ValidID(id) {
			return 0, fmt.Errorf("id no válido: %q", id)
		}
	}
	n := 0
	for _, id := range ids {
		p := filepath.Join(dir, id+ext)
		fi, err := os.Lstat(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return n, err
		}
		if !fi.Mode().IsRegular() {
			continue
		}
		err = os.Remove(p)
		if err == nil {
			n++
		} else if !errors.Is(err, os.ErrNotExist) {
			return n, err
		}
	}
	return n, nil
}

// DeleteAllDir borra todos los <id>.json de dir y deja el resto intacto.
func DeleteAllDir(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if _, ok := idFromName(e.Name()); !ok || !e.Type().IsRegular() {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			return n, err
		}
		n++
	}
	return n, nil
}
