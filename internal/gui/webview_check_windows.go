package gui

import (
	"errors"

	"golang.org/x/sys/windows/registry"
)

const webViewClientKey = `\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`

// CheckWebView2 consulta EdgeUpdate (HKLM/HKCU, vistas de 64 y 32 bits). Es solo
// orientativo: no instalado = clave o valor ausente; otros errores = desconocido
// (runtime de versión fija o sin EdgeUpdate también pueden funcionar).
func CheckWebView2() RuntimeStatus {
	paths := []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node` + webViewClientKey},
		{registry.LOCAL_MACHINE, `SOFTWARE` + webViewClientKey},
		{registry.CURRENT_USER, `SOFTWARE\WOW6432Node` + webViewClientKey},
		{registry.CURRENT_USER, `SOFTWARE` + webViewClientKey},
	}
	var sts []RuntimeStatus
	for _, p := range paths {
		k, err := registry.OpenKey(p.root, p.path, registry.QUERY_VALUE)
		if err != nil {
			sts = append(sts, probeStatus(err))
			continue
		}
		pv, _, err := k.GetStringValue("pv")
		k.Close()
		switch {
		case err != nil:
			sts = append(sts, probeStatus(err))
		case VersionInstalled(pv):
			sts = append(sts, RuntimeInstalled)
		default:
			sts = append(sts, RuntimeMissing)
		}
	}
	return CombineStatus(sts...)
}

func probeStatus(err error) RuntimeStatus {
	if errors.Is(err, registry.ErrNotExist) {
		return RuntimeMissing
	}
	return RuntimeUnknown
}
