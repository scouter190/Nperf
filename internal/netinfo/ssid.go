package netinfo

import "unicode"

// ValidSSID indica si s es un SSID válido: no vacío, ≤ 32 bytes y sin caracteres de control.
func ValidSSID(s string) bool {
	if s == "" || len(s) > 32 {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || r == unicode.ReplacementChar {
			return false
		}
	}
	return true
}
