package gui

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmClose        = 0x0010
	wmCommand      = 0x0111
	wmContextMenu  = 0x007B
	wmLButtonDbl   = 0x0203
	wmRButtonUp    = 0x0205
	wmTrayCallback = 0x8000 + 1  // WM_APP+1
	gwlpWndProc    = ^uintptr(3) // GWLP_WNDPROC (-4)
	swHide         = 0
	swShow         = 5
	swRestore      = 9
	nimAdd         = 0
	nimDelete      = 2
	nifMessage     = 0x1
	nifIcon        = 0x2
	nifTip         = 0x4
	mfString       = 0
	tpmRightButton = 0x2
	tpmReturnCmd   = 0x100
	idiApplication = 32512
	imageIcon      = 1
)

var (
	user32  = windows.NewLazySystemDLL("user32.dll")
	shell32 = windows.NewLazySystemDLL("shell32.dll")

	pShellNotifyIcon       = shell32.NewProc("Shell_NotifyIconW")
	pRegisterClassEx       = user32.NewProc("RegisterClassExW")
	pCreateWindowEx        = user32.NewProc("CreateWindowExW")
	pDestroyWindow         = user32.NewProc("DestroyWindow")
	pDefWindowProc         = user32.NewProc("DefWindowProcW")
	pCallWindowProc        = user32.NewProc("CallWindowProcW")
	pSetWindowLongPtr      = user32.NewProc("SetWindowLongPtrW")
	pShowWindow            = user32.NewProc("ShowWindow")
	pSetForegroundWindow   = user32.NewProc("SetForegroundWindow")
	pCreatePopupMenu       = user32.NewProc("CreatePopupMenu")
	pAppendMenu            = user32.NewProc("AppendMenuW")
	pTrackPopupMenu        = user32.NewProc("TrackPopupMenu")
	pDestroyMenu           = user32.NewProc("DestroyMenu")
	pGetCursorPos          = user32.NewProc("GetCursorPos")
	pLoadImage             = user32.NewProc("LoadImageW")
	pLoadIcon              = user32.NewProc("LoadIconW")
	pPostMessage           = user32.NewProc("PostMessageW")
	pRegisterWindowMessage = user32.NewProc("RegisterWindowMessageW")
)

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     windows.Handle
}

type point struct{ X, Y int32 }

type notifyIconData struct {
	Size            uint32
	Wnd             uintptr
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            uintptr
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GUIDItem        windows.GUID
	BalloonIcon     uintptr
}

// tray es el icono de la bandeja. Vive en el hilo de la UI de go-webview2: su ventana
// de mensajes la atiende el mismo bucle (GetMessage entrega a todas las ventanas del hilo).
type tray struct {
	ctl        *TrayController
	hwnd       uintptr // ventana de mensajes oculta
	main       uintptr // ventana del webview
	oldProc    uintptr
	icon       uintptr
	taskbarMsg uintptr
	mainProcCb uintptr
	trayProcCb uintptr
	iconAdded  bool
}

// Una sola bandeja por proceso: los callbacks de Win32 no admiten cierres con estado.
var theTray *tray

// installTray crea el icono, la ventana de mensajes y la subclase de la ventana
// principal (WM_CLOSE oculta en lugar de cerrar). Debe llamarse en el hilo de la UI.
func installTray(main uintptr, ctl *TrayController) (*tray, error) {
	t := &tray{ctl: ctl, main: main}
	theTray = t

	var hinst windows.Handle
	_ = windows.GetModuleHandleEx(0, nil, &hinst)
	t.icon, _, _ = pLoadImage.Call(uintptr(hinst), 1, imageIcon, 0, 0, 0x8000|0x40) // LR_SHARED|LR_DEFAULTSIZE
	if t.icon == 0 {
		t.icon, _, _ = pLoadIcon.Call(0, idiApplication)
	}
	t.trayProcCb = syscall.NewCallback(trayWndProc)
	t.mainProcCb = syscall.NewCallback(mainWndProc)

	cls, _ := windows.UTF16PtrFromString("NPerfTray")
	wc := wndClassEx{Instance: hinst, WndProc: t.trayProcCb, ClassName: cls}
	wc.Size = uint32(unsafe.Sizeof(wc))
	pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	t.hwnd, _, _ = pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(cls)), 0, 0, 0, 0, 0, 0,
		0, 0, uintptr(hinst), 0) // top-level oculto: recibe el broadcast TaskbarCreated
	if t.hwnd == 0 {
		theTray = nil
		return nil, syscall.GetLastError()
	}
	msg, _ := windows.UTF16PtrFromString("TaskbarCreated")
	t.taskbarMsg, _, _ = pRegisterWindowMessage.Call(uintptr(unsafe.Pointer(msg)))

	t.addIcon()
	// Subclase de la ventana del webview.
	t.oldProc, _, _ = pSetWindowLongPtr.Call(main, gwlpWndProc, t.mainProcCb)
	return t, nil
}

func (t *tray) data() *notifyIconData {
	d := &notifyIconData{Wnd: t.hwnd, ID: 1, Flags: nifMessage | nifIcon | nifTip,
		CallbackMessage: wmTrayCallback, Icon: t.icon}
	d.Size = uint32(unsafe.Sizeof(*d))
	tip, _ := windows.UTF16FromString(TrayTitle)
	copy(d.Tip[:], tip)
	return d
}

func (t *tray) addIcon() {
	r, _, _ := pShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(t.data())))
	t.iconAdded = r != 0
}

// remove quita el icono (NIM_DELETE), restaura la subclase y destruye la ventana de mensajes.
func (t *tray) remove() {
	if t.iconAdded {
		pShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(t.data())))
		t.iconAdded = false
	}
	if t.oldProc != 0 {
		pSetWindowLongPtr.Call(t.main, gwlpWndProc, t.oldProc)
		t.oldProc = 0
	}
	if t.hwnd != 0 {
		pDestroyWindow.Call(t.hwnd)
		t.hwnd = 0
	}
}

func (t *tray) showMain() {
	pShowWindow.Call(t.main, swShow)
	pShowWindow.Call(t.main, swRestore)
	pSetForegroundWindow.Call(t.main)
}

func (t *tray) hideMain() { pShowWindow.Call(t.main, swHide) }

func (t *tray) popup() {
	m, _, _ := pCreatePopupMenu.Call()
	if m == 0 {
		return
	}
	defer pDestroyMenu.Call(m)
	open, _ := windows.UTF16PtrFromString(TrayOpenText)
	quit, _ := windows.UTF16PtrFromString(TrayQuitText)
	pAppendMenu.Call(m, mfString, trayCmdOpen, uintptr(unsafe.Pointer(open)))
	pAppendMenu.Call(m, mfString, trayCmdQuit, uintptr(unsafe.Pointer(quit)))
	var p point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	// Sin SetForegroundWindow el menú no se cierra al hacer clic fuera.
	pSetForegroundWindow.Call(t.hwnd)
	cmd, _, _ := pTrackPopupMenu.Call(m, tpmRightButton|tpmReturnCmd, uintptr(p.X), uintptr(p.Y), 0, t.hwnd, 0)
	pPostMessage.Call(t.hwnd, 0, 0, 0) // WM_NULL: cierra bien el menú
	if cmd != 0 {
		t.ctl.OnCommand(int(cmd))
	}
}

func trayWndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	t := theTray
	if t != nil {
		switch {
		case msg == wmTrayCallback:
			var e TrayEvent
			switch lparam & 0xFFFF {
			case wmLButtonDbl:
				e = TrayDoubleClick
			case wmRButtonUp, wmContextMenu:
				e = TrayContextMenu
			}
			if e != 0 && t.ctl.OnIcon(e) {
				t.popup()
			}
			return 0
		case msg == wmCommand:
			t.ctl.OnCommand(int(wparam & 0xFFFF))
			return 0
		case t.taskbarMsg != 0 && msg == t.taskbarMsg:
			t.addIcon() // el Explorador se reinició: volver a poner el icono
			return 0
		}
	}
	r, _, _ := pDefWindowProc.Call(hwnd, msg, wparam, lparam)
	return r
}

func mainWndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	t := theTray
	if t == nil {
		r, _, _ := pDefWindowProc.Call(hwnd, msg, wparam, lparam)
		return r
	}
	if msg == wmClose && t.ctl.OnClose() {
		return 0
	}
	r, _, _ := pCallWindowProc.Call(t.oldProc, hwnd, msg, wparam, lparam)
	return r
}
