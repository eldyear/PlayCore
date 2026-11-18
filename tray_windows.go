//go:build windows

package main

import (
	"log"
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

const (
	hwndMessage = ^uintptr(2) // HWND_MESSAGE = -3

	WM_LBUTTONUP = 0x0202
	WM_RBUTTONUP = 0x0205
	WM_COMMAND   = 0x0111
	WM_DESTROY   = 0x0002
	WM_NULL      = 0x0000

	WM_TRAY = 0x0400 + 100 // WM_USER + 100

	NIM_ADD    = 0x00000000
	NIM_DELETE = 0x00000002

	NIF_MESSAGE = 0x00000001
	NIF_ICON    = 0x00000002
	NIF_TIP     = 0x00000004

	MF_STRING    = 0x00000000
	MF_SEPARATOR = 0x00000800

	TPM_RIGHTBUTTON = 0x0002
	TPM_RETURNCMD   = 0x0100

	MENU_OPEN   = 1
	MENU_TOGGLE = 2
	MENU_QUIT   = 3
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")

	procRegisterClassEx = user32.NewProc("RegisterClassExW")
	procCreateWindowEx  = user32.NewProc("CreateWindowExW")
	procDefWindowProc   = user32.NewProc("DefWindowProcW")
	procGetMessage      = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessage = user32.NewProc("DispatchMessageW")
	procPostQuitMessage = user32.NewProc("PostQuitMessage")
	procDestroyWindow   = user32.NewProc("DestroyWindow")
	procGetModuleHandle = kernel32.NewProc("GetModuleHandleW")

	procShellNotifyIcon = shell32.NewProc("Shell_NotifyIconW")
	procExtractIconEx   = shell32.NewProc("ExtractIconExW")

	procCreatePopupMenu = user32.NewProc("CreatePopupMenu")
	procAppendMenu      = user32.NewProc("AppendMenuW")
	procTrackPopupMenu  = user32.NewProc("TrackPopupMenu")
	procSetForeground   = user32.NewProc("SetForegroundWindow")
	procPostMessage     = user32.NewProc("PostMessageW")
	procDestroyMenu     = user32.NewProc("DestroyMenu")
	procGetCursorPos    = user32.NewProc("GetCursorPos")
)

type WNDCLASSEX struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     syscall.Handle
	HIcon         syscall.Handle
	HCursor       syscall.Handle
	HbrBackground syscall.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       syscall.Handle
}

type POINT struct {
	X int32
	Y int32
}

type MSG struct {
	HWnd     syscall.Handle
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       POINT
	LPrivate uint32
}

type NOTIFYICONDATA struct {
	CbSize           uint32
	HWnd             syscall.Handle
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            syscall.Handle
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UTimeoutOrVer    uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     syscall.Handle
}

var (
	trayHwnd syscall.Handle
	trayIcon syscall.Handle
	trayCtrl *ServerController
	trayURL  string
)

// runTray показывает иконку в трее и запускает цикл сообщений.
func runTray(controller *ServerController, url string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC в runTray: %v", r)
		}
	}()

	trayCtrl = controller
	trayURL = url

	className, err := syscall.UTF16PtrFromString("PlayCoreTrayWnd")
	if err != nil {
		log.Printf("ошибка className: %v", err)
		return
	}
	hInst, _, _ := procGetModuleHandle.Call(0)
	if hInst == 0 {
		log.Printf("GetModuleHandle вернул 0")
		return
	}

	var wc WNDCLASSEX
	wc.CbSize = uint32(unsafe.Sizeof(wc))
	wc.LpfnWndProc = syscall.NewCallback(trayWndProc)
	wc.HInstance = syscall.Handle(hInst)
	wc.LpszClassName = className

	atom, _, _ := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		log.Printf("RegisterClassEx не удался")
		return
	}

	hwndR, _, _ := procCreateWindowEx.Call(
		0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(className)), 0,
		0, 0, 0, 0,
		hwndMessage,
		0, hInst, 0)
	if hwndR == 0 {
		log.Printf("CreateWindowEx не удался")
		return
	}
	trayHwnd = syscall.Handle(hwndR)
	defer procDestroyWindow.Call(uintptr(trayHwnd))

	trayIcon = loadIcon()
	addTrayIcon()
	defer removeTrayIcon()

	log.Printf("трей запущен, сервер=%v", trayCtrl.IsRunning())

	var msg MSG
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func trayWndProc(hwnd syscall.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_TRAY:
		if lParam == WM_LBUTTONUP {
			openBrowser(trayURL)
		} else if lParam == WM_RBUTTONUP {
			showTrayMenu()
		}
		return 0
	case WM_COMMAND:
		switch uint32(wParam) & 0xFFFF {
		case MENU_OPEN:
			openBrowser(trayURL)
		case MENU_TOGGLE:
			toggleServer()
		case MENU_QUIT:
			procPostQuitMessage.Call(0)
		}
		return 0
	case WM_DESTROY:
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProc.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return r
}

func showTrayMenu() {
	menu, _, _ := procCreatePopupMenu.Call()

	toggleLabel := "Остановить сервер"
	if !trayCtrl.IsRunning() {
		toggleLabel = "Запустить сервер"
	}

	appendMenu(menu, MF_STRING, MENU_OPEN, "Открыть админку")
	appendMenu(menu, MF_STRING, MENU_TOGGLE, toggleLabel)
	appendMenu(menu, MF_SEPARATOR, 0, "")
	appendMenu(menu, MF_STRING, MENU_QUIT, "Выход")

	var pt POINT
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))

	procSetForeground.Call(uintptr(trayHwnd))
	cmd, _, _ := procTrackPopupMenu.Call(menu, TPM_RIGHTBUTTON|TPM_RETURNCMD, uintptr(pt.X), uintptr(pt.Y), 0, uintptr(trayHwnd), 0)
	procPostMessage.Call(uintptr(trayHwnd), WM_NULL, 0, 0)
	procDestroyMenu.Call(menu)

	if cmd != 0 {
		trayWndProc(trayHwnd, WM_COMMAND, cmd, 0)
	}
}

func appendMenu(menu uintptr, flags uintptr, id uintptr, text string) {
	if text == "" {
		procAppendMenu.Call(menu, flags, id, 0)
		return
	}
	p, _ := syscall.UTF16PtrFromString(text)
	procAppendMenu.Call(menu, flags, id, uintptr(unsafe.Pointer(p)))
}

func toggleServer() {
	if trayCtrl.IsRunning() {
		trayCtrl.Stop()
	} else {
		trayCtrl.Start("0.0.0.0:"+getPort(), buildMux(), "", "")
	}
}

func loadIcon() syscall.Handle {
	exe, err := os.Executable()
	if err != nil {
		return 0
	}
	exePtr, err := syscall.UTF16PtrFromString(exe)
	if err != nil {
		return 0
	}
	var large, small syscall.Handle
	r, _, _ := procExtractIconEx.Call(uintptr(unsafe.Pointer(exePtr)), 0, uintptr(unsafe.Pointer(&large)), uintptr(unsafe.Pointer(&small)), 1)
	if r == 0 {
		return 0
	}
	if small != 0 {
		return small
	}
	return large
}

func addTrayIcon() {
	var nid NOTIFYICONDATA
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = trayHwnd
	nid.UID = 1
	nid.UFlags = NIF_MESSAGE | NIF_ICON | NIF_TIP
	nid.UCallbackMessage = WM_TRAY
	nid.HIcon = trayIcon
	copy(nid.SzTip[:], syscall.StringToUTF16("PlayCore — сервер"))
	r, _, _ := procShellNotifyIcon.Call(NIM_ADD, uintptr(unsafe.Pointer(&nid)))
	if r == 0 {
		log.Printf("Shell_NotifyIcon(NIM_ADD) не удался (icon=%v)", trayIcon)
	} else {
		log.Printf("трей-иконка добавлена (icon=%v)", trayIcon)
	}
}

func removeTrayIcon() {
	var nid NOTIFYICONDATA
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = trayHwnd
	nid.UID = 1
	procShellNotifyIcon.Call(NIM_DELETE, uintptr(unsafe.Pointer(&nid)))
}

func openBrowser(url string) {
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}
