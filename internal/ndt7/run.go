package ndt7

import (
	"context"
	"errors"
	"time"

	"nperf/internal/ws"
)

// ErrNoConsent: Run se llamó sin consentimiento explícito.
var ErrNoConsent = errors.New("falta el consentimiento para la prueba con M-Lab")

// RunOptions configura Run. Todo es inyectable para los tests.
type RunOptions struct {
	// Consent debe ser true en cada llamada. Run no muestra el aviso: el
	// llamador (UI o CLI) lo muestra con Notice() y obtiene el consentimiento
	// antes, y garantiza que Consent solo es true si el usuario aceptó.
	Consent bool
	Quota   *Quota
	Locate  LocateOptions
	WS      ws.Options
	// Measure aporta Now, MaxTotal, UploadTime y MinNormal (URL y WS se ignoran).
	Measure MeasureOptions
	Now     func() time.Time // por defecto time.Now (fecha de la cuota y del resultado)
}

// Run hace una prueba completa: consentimiento, cuota, locate, descarga y
// subida (una conexión por sentido, sin reintentos). Un fallo en un sentido
// no aborta el otro (queda "no disponible" con motivo en el Result).
// Devuelve error, junto con un Result con ambos sentidos no disponibles, si
// no llegó a medir: ErrNoConsent, ErrQuotaExceeded o fallo de E/S de la cuota
// (sin petición alguna a locate), o *LocateError.
func Run(ctx context.Context, o RunOptions) (Result, error) {
	nowf := o.Now
	if nowf == nil {
		nowf = time.Now
	}
	now := nowf()
	r := Result{Time: now.Format(time.RFC3339)}
	fail := func(err error, reason string) (Result, error) {
		r.Download = DirResult{Status: StatusUnavailable, Reason: reason}
		r.Upload = DirResult{Status: StatusUnavailable, Reason: reason}
		return r, err
	}
	if !o.Consent {
		return fail(ErrNoConsent, ErrNoConsent.Error())
	}
	if o.Quota == nil {
		err := errors.New("no se pudo comprobar el límite diario")
		return fail(err, err.Error())
	}
	n, err := o.Quota.Reserve(now)
	r.TestsToday = n
	if err != nil {
		return fail(err, err.Error())
	}
	t, err := Locate(ctx, o.Locate)
	if err != nil {
		return fail(err, err.Error())
	}
	r.Server = &Server{Machine: t.Machine, City: t.City, Country: t.Country}

	m := o.Measure
	m.WS = o.WS
	m.URL = t.DownloadURL
	r.Download = dirResult(Download(ctx, m))
	m.URL = t.UploadURL
	r.Upload = dirResult(Upload(ctx, m))
	return r, nil
}

func dirResult(d Direction, err error) DirResult {
	if err != nil {
		return DirResult{Status: StatusUnavailable, Reason: "no se pudo conectar con el servidor de M-Lab: " + err.Error()}
	}
	st := StatusOK
	if d.Incomplete {
		st = StatusIncomplete
	}
	return DirResult{Status: st, Reason: d.Reason, Direction: &d}
}
