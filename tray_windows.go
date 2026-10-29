package main

// Icon in the notification area (next to the clock), drawn by the agent – it runs in the user's session.
// Tooltip: MQTT connection and Leonzilla mode (with the sleep countdown). Menu: open the settings window,
// monitors off, lock, hide the icon until the next sign-in. Left or right click = the menu.

import (
	"os"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procShellNotifyIcon     = shell32.NewProc("Shell_NotifyIconW")
	procRegisterClassEx     = user32.NewProc("RegisterClassExW")
	procCreateWindowEx      = user32.NewProc("CreateWindowExW")
	procDefWindowProc       = user32.NewProc("DefWindowProcW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procGetMessage          = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessage     = user32.NewProc("DispatchMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procRegisterWindowMsg   = user32.NewProc("RegisterWindowMessageW")
	procLoadImage           = user32.NewProc("LoadImageW")
	procGetSystemMetrics    = user32.NewProc("GetSystemMetrics")
	procCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	procAppendMenu          = user32.NewProc("AppendMenuW")
	procSetMenuDefaultItem  = user32.NewProc("SetMenuDefaultItem")
	procTrackPopupMenu      = user32.NewProc("TrackPopupMenu")
	procDestroyMenu         = user32.NewProc("DestroyMenu")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procSetTimer            = user32.NewProc("SetTimer")
	procGetModuleHandle     = kernel32.NewProc("GetModuleHandleW")
)

const (
	wmDestroy     = 0x0002
	wmNull        = 0x0000
	wmTimer       = 0x0113
	ninSelect     = 0x0400 // WM_USER
	ninKeySelect  = 0x0401
	wmContextMenu = 0x007B
	wmApp         = 0x8000
	wmTray        = wmApp + 1

	nimAdd        = 0
	nimModify     = 1
	nimDelete     = 2
	nimSetVersion = 4
	nifMessage    = 0x1
	nifIcon       = 0x2
	nifTip        = 0x4
	nifShowTip    = 0x80

	mfString    = 0x0
	mfGrayed    = 0x1
	mfSeparator = 0x800
	tpmRetCmd   = 0x0100
	tpmRightBtn = 0x0002

	idOpen    = 101
	idMonitor = 102
	idLock    = 103
	idHide    = 104
)

type notifyIconData struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         windows.GUID
	HBalloonIcon     uintptr
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type winMsg struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

type tray struct {
	a          *agent
	hwnd       uintptr
	icon       uintptr
	nid        notifyIconData
	taskbarMsg uint32
}

// runTray: blocks (own OS thread with a message loop) – call with go.
func runTray(a *agent) {
	if os.Getenv("LEOLINK_NOTRAY") == "1" {
		return
	}
	runtime.LockOSThread()
	t := &tray{a: a}
	hinst, _, _ := procGetModuleHandle.Call(0)
	cx, _, _ := procGetSystemMetrics.Call(49) // SM_CXSMICON
	cy, _, _ := procGetSystemMetrics.Call(50)
	t.icon, _, _ = procLoadImage.Call(hinst, 1, 1 /*IMAGE_ICON*/, cx, cy, 0) // icon group 1 from tools/mkres
	cls, _ := windows.UTF16PtrFromString("LeoLinkTray")
	wc := wndClassEx{WndProc: windows.NewCallback(t.wndProc), Instance: hinst, ClassName: cls}
	wc.Size = uint32(unsafe.Sizeof(wc))
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	title, _ := windows.UTF16PtrFromString("LeoLink")
	// an ordinary hidden top-level window (a message-only one would not get the "TaskbarCreated" broadcast)
	t.hwnd, _, _ = procCreateWindowEx.Call(0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(title)), 0, 0, 0, 0, 0, 0, 0, hinst, 0)
	if t.hwnd == 0 {
		logf("ikona u hodin: okno nejde vytvořit")
		return
	}
	tbc, _ := windows.UTF16PtrFromString("TaskbarCreated")
	r, _, _ := procRegisterWindowMsg.Call(uintptr(unsafe.Pointer(tbc)))
	t.taskbarMsg = uint32(r)
	t.add()
	procSetTimer.Call(t.hwnd, 1, 5000, 0)
	var m winMsg
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func (t *tray) add() {
	t.nid = notifyIconData{HWnd: t.hwnd, UID: 1, UFlags: nifMessage | nifIcon | nifTip | nifShowTip, UCallbackMessage: wmTray, HIcon: t.icon}
	t.nid.CbSize = uint32(unsafe.Sizeof(t.nid))
	t.setTip()
	procShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&t.nid)))
	t.nid.UVersion = 4 // NOTIFYICON_VERSION_4
	procShellNotifyIcon.Call(nimSetVersion, uintptr(unsafe.Pointer(&t.nid)))
}

func (t *tray) setTip() {
	s := t.a.trayStatus()
	u, _ := windows.UTF16FromString(s)
	if len(u) > len(t.nid.SzTip) {
		u = u[:len(t.nid.SzTip)-1]
		u = append(u, 0)
	}
	t.nid.SzTip = [128]uint16{}
	copy(t.nid.SzTip[:], u)
}

func (t *tray) wndProc(hwnd, m, wp, lp uintptr) uintptr {
	msg := uint32(m)
	switch {
	case msg == wmTray:
		switch uint32(lp & 0xFFFF) { // NOTIFYICON_VERSION_4: the event is in the low word
		case ninSelect, ninKeySelect, wmContextMenu: // left click / Enter / right click (each comes once)
			t.menu()
		}
		return 0
	case msg == wmTimer:
		t.setTip()
		t.nid.UFlags = nifTip | nifShowTip
		procShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&t.nid)))
		return 0
	case msg == t.taskbarMsg && t.taskbarMsg != 0: // Explorer restarted – the icon is gone
		t.add()
		return 0
	case msg == wmDestroy:
		procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&t.nid)))
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProc.Call(hwnd, uintptr(msg), wp, lp)
	return r
}

func (t *tray) menu() {
	m, _, _ := procCreatePopupMenu.Call()
	defer procDestroyMenu.Call(m)
	add := func(flags uint32, id int, text string) {
		p, _ := windows.UTF16PtrFromString(text)
		procAppendMenu.Call(m, uintptr(flags), uintptr(id), uintptr(unsafe.Pointer(p)))
	}
	for _, line := range strings.Split(t.a.trayStatus(), "\n") {
		add(mfString|mfGrayed, 0, line)
	}
	add(mfSeparator, 0, "")
	add(mfString, idOpen, T("Otevřít nastavení LeoLink…"))
	add(mfString, idMonitor, T("Vypnout monitory"))
	add(mfString, idLock, T("Zamknout"))
	add(mfSeparator, 0, "")
	add(mfString, idHide, T("Skrýt ikonu (do příštího přihlášení)"))
	procSetMenuDefaultItem.Call(m, idOpen, 0)
	var pt struct{ X, Y int32 }
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(t.hwnd)
	cmd, _, _ := procTrackPopupMenu.Call(m, tpmRetCmd|tpmRightBtn, uintptr(pt.X), uintptr(pt.Y), 0, t.hwnd, 0)
	procPostMessage.Call(t.hwnd, wmNull, 0, 0)
	switch int(cmd) {
	case idOpen:
		openSettings()
	case idMonitor:
		go func() {
			if err := sys.Monitor(false); err != nil {
				logf("monitor_off: %v", err)
			}
		}()
	case idLock:
		_ = sys.Lock()
	case idHide:
		logf("ikona u hodin skrytá (do příštího přihlášení)")
		procDestroyWindow.Call(t.hwnd)
	}
}

// openSettings starts the settings window (LeoLink.exe without arguments asks for administrator rights itself).
func openSettings() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	if err := windows.ShellExecute(0, windows.StringToUTF16Ptr("open"), windows.StringToUTF16Ptr(exe), nil, nil, windows.SW_SHOWNORMAL); err != nil {
		logf("nastavení nejde otevřít: %v", err)
	}
}
