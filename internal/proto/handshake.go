package proto

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"nperf/internal/auth"
	"nperf/internal/version"
)

// ClientHandshake autentica en ambos sentidos y devuelve el id de sesión emitido
// tras la autenticación. Secuencia:
//
//	C: hello{version}  S: hello{version, nonce}
//	C: auth{nonce, mac} S: auth{mac, session}
//
// El servidor demuestra conocer la clave (el cliente no acepta resultados de un
// servidor sin clave). Con versión mayor distinta devuelve un error de incompatibilidad.
func ClientHandshake(c *Codec, key, ver string) (string, error) {
	if err := c.Write(Message{Type: TypeHello, Version: ver}); err != nil {
		return "", err
	}
	h, err := readTyped(c, TypeHello)
	if err != nil {
		return "", err
	}
	if ok, verr := version.Compatible(ver, h.Version); verr != nil || !ok {
		return "", fmt.Errorf("%s: cliente %s, servidor %q", ErrIncompatible, ver, h.Version)
	}
	cn, err := auth.NewNonce()
	if err != nil {
		return "", err
	}
	if h.Nonce == "" {
		return "", errors.New("el servidor no envió desafío")
	}
	if err := c.Write(Message{Type: TypeAuth, Nonce: cn, MAC: auth.Sum(key, auth.RoleClient, h.Nonce, cn)}); err != nil {
		return "", err
	}
	r, err := readTyped(c, TypeAuth)
	if err != nil {
		return "", err
	}
	if !auth.Verify(key, auth.RoleServer, r.MAC, h.Nonce, cn, r.Session) {
		return "", errors.New("el servidor no demostró conocer la clave: " + ErrBadKey)
	}
	if r.Session == "" {
		return "", errors.New("el servidor no emitió sesión")
	}
	return r.Session, nil
}

// ServerHandshake es la contraparte; devuelve el id de sesión autenticada.
// Ante clave o versión inválidas envía un mensaje de error al cliente, y el error
// devuelto describe el motivo para el registro del servidor.
// h es el hello del cliente, ya leído.
func ServerHandshake(c *Codec, h Message, key, ver string) (string, error) {
	fail := func(msg string) (string, error) {
		c.Write(Message{Type: TypeError, Error: msg})
		return "", errors.New(msg)
	}
	if ok, verr := version.Compatible(ver, h.Version); verr != nil || !ok {
		return fail(fmt.Sprintf("%s: servidor %s, cliente %q", ErrIncompatible, ver, h.Version))
	}
	sn, err := auth.NewNonce()
	if err != nil {
		return "", err
	}
	if err := c.Write(Message{Type: TypeHello, Version: ver, Nonce: sn}); err != nil {
		return "", err
	}
	a, err := readTyped(c, TypeAuth)
	if err != nil {
		return "", err
	}
	if a.Nonce == "" || !auth.Verify(key, auth.RoleClient, a.MAC, sn, a.Nonce) {
		return fail(ErrBadKey)
	}
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := hex.EncodeToString(b)
	if err := c.Write(Message{Type: TypeAuth, MAC: auth.Sum(key, auth.RoleServer, sn, a.Nonce, id), Session: id}); err != nil {
		return "", err
	}
	return id, nil
}

func readTyped(c *Codec, typ string) (Message, error) {
	m, err := c.Read()
	if err != nil {
		return m, fmt.Errorf("canal de control cerrado: %w", err)
	}
	if m.Type == TypeError {
		return m, fmt.Errorf("el servidor respondió: %s", m.Error)
	}
	if m.Type != typ {
		return m, fmt.Errorf("mensaje inesperado %q (se esperaba %q)", m.Type, typ)
	}
	return m, nil
}
