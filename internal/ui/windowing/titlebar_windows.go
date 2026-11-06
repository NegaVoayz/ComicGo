//go:build windows

package windowing

import (
	"log"
	"sync"
	"unsafe"

	"gioui.org/app"
	"golang.org/x/sys/windows"
)

const (
	wmNull            = 0x0000
	wmContextMenu     = 0x007b
	wmNCHitTest       = 0x0084
	wmNCMouseMove     = 0x00a0
	wmNCLButtonDown   = 0x00a1
	wmNCLButtonUp     = 0x00a2
	wmNCLButtonDblClk = 0x00a3
	wmNCRButtonUp     = 0x00a5
	wmSysKeyDown      = 0x0104
	wmSysCommand      = 0x0112
	wmRButtonUp       = 0x0205
	wmNCMouseLeave    = 0x02a2
	htClient          = 1
	htMinButton       = 8
	htMaxButton       = 9
	htClose           = 20
	tmeLeave          = 0x00000002
	tmeNonClient      = 0x00000010
	tpmRightButton    = 0x0002
	tpmReturnCmd      = 0x0100
	vkSpace           = 0x20
)

type trackMouseEvent struct {
	Size      uint32
	Flags     uint32
	Track     windows.Handle
	HoverTime uint32
}

type winPoint struct {
	X int32
	Y int32
}

type winRect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type windowsWindowChromeBackend struct {
	post func(any)

	mu         sync.RWMutex
	hwnd       uintptr
	oldWndProc uintptr
	regions    chromeHitRegions

	dropTarget     any
	dropHWND       uintptr
	oleInitialized bool
}

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procSetWindowLongPtr    = user32.NewProc("SetWindowLongPtrW")
	procCallWindowProc      = user32.NewProc("CallWindowProcW")
	procTrackMouseEvent     = user32.NewProc("TrackMouseEvent")
	procScreenToClient      = user32.NewProc("ScreenToClient")
	procClientToScreen      = user32.NewProc("ClientToScreen")
	procGetWindowRect       = user32.NewProc("GetWindowRect")
	procGetSystemMenu       = user32.NewProc("GetSystemMenu")
	procTrackPopupMenu      = user32.NewProc("TrackPopupMenu")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procSendMessage         = user32.NewProc("SendMessageW")
	procPostMessage         = user32.NewProc("PostMessageW")
	chromeSubclassCallback  = windows.NewCallback(chromeWindowProc)
	chromeBackendsByWindow  sync.Map
)

func newWindowChromeBackend(post func(any)) windowChromeBackend {
	return &windowsWindowChromeBackend{post: post}
}

func (b *windowsWindowChromeBackend) handlePlatformEvent(ev any) bool {
	view, ok := ev.(app.Win32ViewEvent)
	if !ok {
		return false
	}
	if view.Valid() {
		b.attach(view.HWND)
	} else {
		b.close()
	}
	return true
}

func (b *windowsWindowChromeBackend) attach(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	b.mu.Lock()
	if b.hwnd == hwnd && b.oldWndProc != 0 {
		b.mu.Unlock()
		return
	}
	b.hwnd = hwnd
	b.mu.Unlock()

	chromeBackendsByWindow.Store(hwnd, b)
	old, _, err := procSetWindowLongPtr.Call(
		hwnd,
		^uintptr(3), // GWLP_WNDPROC
		chromeSubclassCallback,
	)
	if old == 0 {
		log.Printf("SetWindowLongPtr failed for HWND %#x: %v", hwnd, err)
		chromeBackendsByWindow.Delete(hwnd)
	} else {
		b.mu.Lock()
		b.oldWndProc = old
		b.mu.Unlock()
	}
	b.attachDrop(hwnd)
}

func (b *windowsWindowChromeBackend) setCaptionRegions(regions chromeHitRegions) {
	b.mu.Lock()
	b.regions = regions
	b.mu.Unlock()
}

func (b *windowsWindowChromeBackend) close() {
	b.mu.Lock()
	hwnd, oldWndProc := b.hwnd, b.oldWndProc
	b.hwnd = 0
	b.oldWndProc = 0
	b.regions = chromeHitRegions{}
	b.mu.Unlock()

	if hwnd != 0 && oldWndProc != 0 {
		procSetWindowLongPtr.Call(hwnd, ^uintptr(3), oldWndProc)
		chromeBackendsByWindow.Delete(hwnd)
	}
	b.detachDrop()
}

func chromeWindowProc(hwnd, message, wParam, lParam, subclassID, refData uintptr) uintptr {
	value, ok := chromeBackendsByWindow.Load(hwnd)
	if !ok {
		return callWindowProc(0, hwnd, message, wParam, lParam)
	}
	b := value.(*windowsWindowChromeBackend)

	switch message {
	case wmNCHitTest:
		part := b.captionPartAtScreen(lParam)
		if part != chromeNone {
			return hitTestForChromePart(part)
		}
	case wmNCRButtonUp:
		if b.captionContainsScreenPoint(lParam) {
			b.showSystemMenu(lParam)
			return 0
		}
	case wmRButtonUp:
		if b.captionContainsClientPoint(lParam) {
			x, y := signedPackedPoint(lParam)
			point := winPoint{X: int32(x), Y: int32(y)}
			procClientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&point)))
			b.showSystemMenu(packWinPoint(point))
			return 0
		}
	case wmContextMenu:
		if b.captionContainsScreenPoint(lParam) {
			b.showSystemMenu(lParam)
			return 0
		}
	case wmSysKeyDown:
		if wParam == vkSpace && lParam&(1<<29) != 0 {
			b.showSystemMenuAtWindowCenter()
			return 0
		}
	case wmNCMouseMove:
		part := chromePartFromHitTest(uint32(wParam))
		b.postHover(part)
		track := trackMouseEvent{
			Size:  uint32(unsafe.Sizeof(trackMouseEvent{})),
			Flags: tmeLeave | tmeNonClient,
			Track: windows.Handle(hwnd),
		}
		procTrackMouseEvent.Call(uintptr(unsafe.Pointer(&track)))
		return 0
	case wmNCMouseLeave:
		b.postHover(chromeNone)
		return 0
	case wmNCLButtonDown, wmNCLButtonDblClk:
		part := chromePartFromHitTest(uint32(wParam))
		if part != chromeNone {
			b.post(ActionEvent{Part: part})
			return 0
		}
	case wmNCLButtonUp:
		if part := chromePartFromHitTest(uint32(wParam)); part != chromeNone {
			return 0
		}
	}
	b.mu.RLock()
	oldWndProc := b.oldWndProc
	b.mu.RUnlock()
	return callWindowProc(oldWndProc, hwnd, message, wParam, lParam)
}
