// Comando release: empaqueta NPerf para publicarlo (zip determinista,
// SHA256SUMS y notas extraídas del CHANGELOG). Solo biblioteca estándar.
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// CopyrightMarker es el marcador que bloquea la publicación.
const CopyrightMarker = "TITULAR-PENDIENTE"

var versionRe = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

// entry describe un archivo del paquete: nombre en el zip y origen.
type entry struct {
	name     string
	src      string
	optional bool
}

func entries(root, exeDir string) []entry {
	return []entry{
		{"nperf.exe", filepath.Join(exeDir, "nperf.exe"), false},
		{"nperf-gui.exe", filepath.Join(exeDir, "nperf-gui.exe"), false},
		{"README.md", filepath.Join(root, "README.md"), false},
		{"privacidad.md", filepath.Join(root, "docs", "privacidad.md"), false},
		{"licencias-terceros.txt", filepath.Join(root, "docs", "licencias-terceros.txt"), false},
		{"LICENSE", filepath.Join(root, "LICENSE"), true},
	}
}

// Notes devuelve la sección "## [version]" del changelog, hasta la
// siguiente línea "## " (o el final). Falla si no existe o está vacía.
func Notes(changelog, version string) (string, error) {
	lines := strings.Split(strings.ReplaceAll(changelog, "\r\n", "\n"), "\n")
	head := "## [" + version + "]"
	start := -1
	for i, l := range lines {
		if l == head || strings.HasPrefix(l, head+" ") {
			start = i
			break
		}
	}
	if start < 0 {
		return "", fmt.Errorf("CHANGELOG sin la sección %s", head)
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			end = i
			break
		}
	}
	body := strings.TrimSpace(strings.Join(lines[start+1:end], "\n"))
	if body == "" {
		return "", fmt.Errorf("la sección %s del CHANGELOG está vacía", head)
	}
	return body + "\n", nil
}

// CheckCopyright rechaza el marcador pendiente en los winres.json.
func CheckCopyright(root string) error {
	for _, c := range []string{"nperf", "nperf-gui"} {
		p := filepath.Join(root, "cmd", c, "winres", "winres.json")
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if bytes.Contains(b, []byte(CopyrightMarker)) {
			return fmt.Errorf("%s: el titular del copyright sigue sin definir (%s)", p, CopyrightMarker)
		}
	}
	return nil
}

// Package crea el zip, SHA256SUMS y notes.md en out. Devuelve el nombre del zip.
func Package(version, root, exeDir, out string) (string, error) {
	if !versionRe.MatchString(version) {
		return "", fmt.Errorf("versión inválida %q (se espera X.Y.Z)", version)
	}
	cl, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if err != nil {
		return "", err
	}
	notes, err := Notes(string(cl), version)
	if err != nil {
		return "", err
	}
	type file struct {
		name string
		data []byte
	}
	var files []file
	for _, e := range entries(root, exeDir) {
		b, err := os.ReadFile(e.src)
		if err != nil {
			if e.optional && errors.Is(err, os.ErrNotExist) {
				continue
			}
			return "", fmt.Errorf("falta el archivo obligatorio %s: %w", e.name, err)
		}
		files = append(files, file{e.name, b})
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	fixed := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, f := range files {
		h := &zip.FileHeader{Name: f.name, Method: zip.Deflate, Modified: fixed}
		h.SetMode(0o644)
		w, err := zw.CreateHeader(h)
		if err != nil {
			return "", err
		}
		if _, err := w.Write(f.data); err != nil {
			return "", err
		}
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return "", err
	}
	zipName := fmt.Sprintf("NPerf-%s-windows-amd64.zip", version)
	if err := os.WriteFile(filepath.Join(out, zipName), buf.Bytes(), 0o644); err != nil {
		return "", err
	}
	sum := sha256.Sum256(buf.Bytes())
	line := hex.EncodeToString(sum[:]) + "  " + zipName + "\n"
	if err := os.WriteFile(filepath.Join(out, "SHA256SUMS"), []byte(line), 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(out, "notes.md"), []byte(notes), 0o644); err != nil {
		return "", err
	}
	return zipName, nil
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("release", flag.ContinueOnError)
	version := fs.String("version", "", "versión X.Y.Z")
	exeDir := fs.String("exe-dir", "", "directorio con nperf.exe y nperf-gui.exe")
	out := fs.String("out", "dist", "directorio de salida")
	root := fs.String("root", ".", "raíz del repositorio")
	check := fs.Bool("check-copyright", false, "rechaza el titular de copyright pendiente")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *check {
		if err := CheckCopyright(*root); err != nil {
			return err
		}
	}
	if *exeDir == "" {
		// Sin exes solo se pueden extraer las notas.
		if *version == "" {
			return errors.New("falta -version")
		}
		cl, err := os.ReadFile(filepath.Join(*root, "CHANGELOG.md"))
		if err != nil {
			return err
		}
		n, err := Notes(string(cl), *version)
		if err != nil {
			return err
		}
		_, err = io.WriteString(stdout, n)
		return err
	}
	name, err := Package(*version, *root, *exeDir, *out)
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, "creado", filepath.Join(*out, name))
	return nil
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "release:", err)
		os.Exit(1)
	}
}
