//go:build !windows

package gui

import "errors"

func init() {
	OpenURL = func(string) error { return errors.New("solo disponible en Windows") }
}
