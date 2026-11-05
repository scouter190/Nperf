// Package auth implementa el desafío-respuesta HMAC-SHA256 del canal de control.
// La clave nunca viaja: solo se envían nonces y MACs.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// Roles que firman, para que una respuesta del cliente no sirva como prueba del servidor.
const (
	RoleClient = "client"
	RoleServer = "server"
)

// NewNonce devuelve 16 bytes aleatorios en hexadecimal.
func NewNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Sum calcula HMAC-SHA256(clave, rol | 0 | parte | 0 ...) en hexadecimal. Las partes son
// los nonces (servidor, cliente) y, en la prueba del servidor, el id de sesión.
func Sum(key, role string, parts ...string) string {
	m := hmac.New(sha256.New, []byte(key))
	for _, s := range append([]string{role}, parts...) {
		m.Write([]byte(s))
		m.Write([]byte{0})
	}
	return hex.EncodeToString(m.Sum(nil))
}

// Verify compara en tiempo constante.
func Verify(key, role, mac string, parts ...string) bool {
	return hmac.Equal([]byte(Sum(key, role, parts...)), []byte(mac))
}
