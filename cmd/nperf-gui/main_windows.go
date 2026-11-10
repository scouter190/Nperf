// Comando nperf-gui: interfaz gráfica de NPerf (ventana WebView2 con la UI embebida).
package main

import (
	"context"
	"fmt"
	"os"

	"nperf/internal/app"
	"nperf/internal/gui"
	"nperf/internal/ui"
)

func main() {
	if err := run(); err != nil {
		gui.ShowStartupError(err)
		os.Exit(1)
	}
}

func run() error {
	release, err := app.Acquire("NPerf-GUI")
	if err != nil {
		if err == app.ErrAlreadyRunning {
			gui.ShowInfo("NPerf ya está abierto")
			return nil
		}
		return err
	}
	defer release()

	cfgRoot, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("no se encontró el directorio de configuración: %w", err)
	}
	dir, webviewDir := gui.Paths(cfgRoot)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc, err := gui.NewService(ctx, dir, nil, nil)
	if err != nil {
		return err
	}
	defer svc.Stop()

	secret, err := gui.NewSecret()
	if err != nil {
		return fmt.Errorf("no se pudo generar el secreto de sesión: %w", err)
	}
	url, closeUI, err := gui.ServeUI(ui.Files(), secret)
	if err != nil {
		return fmt.Errorf("no se pudo iniciar la interfaz: %w", err)
	}
	defer closeUI()

	b := &gui.Bridge{Secret: secret, B: svc.Backend(), OnConfigChange: func() { svc.RestartAsync() }}
	return gui.RunWindow(url, webviewDir, b.Call)
}
