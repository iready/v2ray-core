//go:build windows

package notify

import (
	"fmt"
	"os"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	hwndMessage = ^uintptr(2) // HWND_MESSAGE = -3

	nimAdd    = 0
	nimModify = 1
	nimDelete = 2

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004
	nifInfo    = 0x00000010

	niifInfo    = 0x00000001
	niifNoSound = 0x00000010

	idiApplication = 32512
)

type notifyIconData struct {
	CbSize           uint32
	Hwnd             windows.HWND
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            windows.Handle
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UTimeout         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         windows.GUID
	HBalloonIcon     windows.Handle
}

var (
	shell32            = windows.NewLazySystemDLL("shell32.dll")
	user32             = windows.NewLazySystemDLL("user32.dll")
	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")
	procExtractIconExW   = shell32.NewProc("ExtractIconExW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDestroyWindow    = user32.NewProc("DestroyWindow")
	procLoadIconW        = user32.NewProc("LoadIconW")
	procPeekMessageW     = user32.NewProc("PeekMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessageW = user32.NewProc("DispatchMessageW")
	procDestroyIcon      = user32.NewProc("DestroyIcon")

	notifyMu sync.Mutex
)

func show(title, body string) error {
	notifyMu.Lock()
	defer notifyMu.Unlock()

	className, err := windows.UTF16PtrFromString("STATIC")
	if err != nil {
		return err
	}
	hwnd, _, callErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		0,
		0,
		0, 0, 0, 0,
		hwndMessage,
		0, 0, 0,
	)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowEx: %w", callErr)
	}
	defer procDestroyWindow.Call(hwnd)

	icon, ownIcon := loadAppIcon()
	if ownIcon && icon != 0 {
		defer procDestroyIcon.Call(uintptr(icon))
	}
	if icon == 0 {
		icon = loadStockIcon()
	}

	nid := notifyIconData{
		CbSize:      uint32(unsafe.Sizeof(notifyIconData{})),
		Hwnd:        windows.HWND(hwnd),
		UID:         1,
		UFlags:      nifMessage | nifIcon | nifTip | nifInfo,
		HIcon:       icon,
		UTimeout:    5000,
		DwInfoFlags: niifInfo | niifNoSound,
	}
	copyUTF16(nid.SzTip[:], "Rocket")
	copyUTF16(nid.SzInfoTitle[:], truncateRunes(title, 63))
	copyUTF16(nid.SzInfo[:], truncateRunes(body, 255))

	if err := shellNotify(nimAdd, &nid); err != nil {
		return err
	}
	defer shellNotify(nimDelete, &nid)

	// 气泡依赖消息泵；泵几秒后收起托盘图标。
	deadline := time.Now().Add(4 * time.Second)
	var msg struct {
		Hwnd    windows.HWND
		Message uint32
		WParam  uintptr
		LParam  uintptr
		Time    uint32
		Pt      struct{ X, Y int32 }
	}
	for time.Now().Before(deadline) {
		ret, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&msg)), hwnd, 0, 0, 1)
		if ret != 0 {
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		} else {
			time.Sleep(50 * time.Millisecond)
		}
	}
	return nil
}

func shellNotify(action uint32, nid *notifyIconData) error {
	r, _, err := procShellNotifyIconW.Call(uintptr(action), uintptr(unsafe.Pointer(nid)))
	if r == 0 {
		return fmt.Errorf("Shell_NotifyIcon(%d): %w", action, err)
	}
	return nil
}

func loadAppIcon() (windows.Handle, bool) {
	exe, err := os.Executable()
	if err != nil {
		return 0, false
	}
	exeW, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return 0, false
	}
	var large, small windows.Handle
	n, _, _ := procExtractIconExW.Call(
		uintptr(unsafe.Pointer(exeW)),
		0,
		uintptr(unsafe.Pointer(&large)),
		uintptr(unsafe.Pointer(&small)),
		1,
	)
	if n == 0 {
		return 0, false
	}
	if large != 0 && small != 0 && large != small {
		procDestroyIcon.Call(uintptr(large))
	}
	if small != 0 {
		return small, true
	}
	if large != 0 {
		return large, true
	}
	return 0, false
}

func loadStockIcon() windows.Handle {
	ico, _, _ := procLoadIconW.Call(0, uintptr(idiApplication))
	return windows.Handle(ico)
}

func copyUTF16(dst []uint16, s string) {
	src, err := windows.UTF16FromString(s)
	if err != nil {
		return
	}
	n := len(src)
	if n > len(dst) {
		n = len(dst)
		src = src[:n]
		src[n-1] = 0
	}
	copy(dst, src)
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
