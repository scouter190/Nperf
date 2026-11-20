//go:build !windows

package gui

import "errors"

// SaveDialog solo existe en Windows.
func SaveDialog(format, suggested string) (string, error) {
	return "", errors.New("solo disponible en Windows")
}
