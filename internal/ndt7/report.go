package ndt7

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Estados de un sentido.
const (
	StatusOK          = "ok"
	StatusIncomplete  = "incomplete"
	StatusUnavailable = "unavailable"
)

// DirResult es un sentido: Direction es nil si no está disponible.
type DirResult struct {
	Status    string
	Reason    string
	Direction *Direction
}

// Server identifica el servidor de M-Lab usado.
type Server struct {
	Machine string `json:"machine"`
	City    string `json:"city,omitempty"`
	Country string `json:"country,omitempty"`
}

// Result es el resultado de Run (JSON v1, command "speed").
type Result struct {
	Time       string
	TestsToday int
	Server     *Server
	Download   DirResult
	Upload     DirResult
}

type jsonResult struct {
	FormatVersion  int        `json:"format_version"`
	Command        string     `json:"command"`
	Time           string     `json:"time"`
	Provider       string     `json:"provider"`
	Attribution    string     `json:"attribution"`
	Server         *Server    `json:"server"`
	TestsToday     int        `json:"tests_today"`
	Download       *Direction `json:"download"`
	DownloadStatus string     `json:"download_status"`
	DownloadReason string     `json:"download_reason,omitempty"`
	Upload         *Direction `json:"upload"`
	UploadStatus   string     `json:"upload_status"`
	UploadReason   string     `json:"upload_reason,omitempty"`
}

// JSON devuelve el resultado en JSON v1 (null + status para lo no medido).
func (r Result) JSON() ([]byte, error) {
	return json.MarshalIndent(jsonResult{
		FormatVersion: 1, Command: "speed", Time: r.Time, Provider: "M-Lab NDT7",
		Attribution: Attribution, Server: r.Server, TestsToday: r.TestsToday,
		Download: r.Download.Direction, DownloadStatus: r.Download.Status, DownloadReason: r.Download.Reason,
		Upload: r.Upload.Direction, UploadStatus: r.Upload.Status, UploadReason: r.Upload.Reason,
	}, "", "  ")
}

// Text devuelve el resultado legible en español, con aviso y atribución.
func (r Result) Text() string {
	var b strings.Builder
	b.WriteString("Velocidad a internet (M-Lab NDT7)\n")
	if r.Server != nil {
		loc := strings.Trim(strings.Join([]string{r.Server.City, r.Server.Country}, ", "), ", ")
		fmt.Fprintf(&b, "Servidor: %s", r.Server.Machine)
		if loc != "" {
			fmt.Fprintf(&b, " (%s)", loc)
		}
		b.WriteString("\n")
	}
	b.WriteString(dirText("Descarga", r.Download))
	b.WriteString(dirText("Subida", r.Upload))
	b.WriteString("\n" + NoticeIP + "\n" + NoticeData + "\n" + Attribution + "\n")
	return b.String()
}

func dirText(name string, d DirResult) string {
	if d.Direction == nil {
		return fmt.Sprintf("%s: no disponible (%s)\n", name, d.Reason)
	}
	s := fmt.Sprintf("%s: %.1f Mbps", name, d.Direction.Mbps)
	if d.Direction.MinRTTms != nil {
		s += fmt.Sprintf(", RTT mínimo %.1f ms", *d.Direction.MinRTTms)
	}
	if d.Status == StatusIncomplete {
		s += " (incompleta: " + d.Reason + ")"
	}
	for _, n := range d.Direction.Notes {
		s += "\n  nota: " + n
	}
	return s + "\n"
}
