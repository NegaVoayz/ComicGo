//go:build windows

package windowing

import (
	"image"
	"unsafe"
)

func callWindowProc(oldWndProc, hwnd, message, wParam, lParam uintptr) uintptr {
	result, _, _ := procCallWindowProc.Call(oldWndProc, hwnd, message, wParam, lParam)
	return result
}

func (b *windowsWindowChromeBackend) captionPartAtScreen(lParam uintptr) chromePart {
	x, y := signedPackedPoint(lParam)
	point := winPoint{X: int32(x), Y: int32(y)}
	b.mu.RLock()
	hwnd := b.hwnd
	regions := b.regions
	b.mu.RUnlock()
	if hwnd != 0 {
		procScreenToClient.Call(hwnd, uintptr(unsafe.Pointer(&point)))
	}
	return regions.at(image.Pt(int(point.X), int(point.Y)))
}

func (b *windowsWindowChromeBackend) captionContainsScreenPoint(lParam uintptr) bool {
	x, y := signedPackedPoint(lParam)
	point := winPoint{X: int32(x), Y: int32(y)}
	b.mu.RLock()
	hwnd := b.hwnd
	caption := b.regions.Caption
	b.mu.RUnlock()
	if hwnd != 0 {
		procScreenToClient.Call(hwnd, uintptr(unsafe.Pointer(&point)))
	}
	return image.Pt(int(point.X), int(point.Y)).In(caption)
}

func (b *windowsWindowChromeBackend) captionContainsClientPoint(lParam uintptr) bool {
	x, y := signedPackedPoint(lParam)
	b.mu.RLock()
	caption := b.regions.Caption
	b.mu.RUnlock()
	return image.Pt(x, y).In(caption)
}

func signedPackedPoint(lParam uintptr) (int, int) {
	return int(int32(int16(lParam & 0xffff))), int(int32(int16((lParam >> 16) & 0xffff)))
}

func packWinPoint(point winPoint) uintptr {
	x := uintptr(uint16(point.X))
	y := uintptr(uint16(point.Y))
	return x | y<<16
}

func chromePartFromHitTest(hit uint32) chromePart {
	switch hit {
	case htMinButton:
		return chromeMinimize
	case htMaxButton:
		return chromeMaximize
	case htClose:
		return chromeClose
	default:
		return chromeNone
	}
}

func hitTestForChromePart(part chromePart) uintptr {
	switch part {
	case chromeMinimize:
		return htMinButton
	case chromeMaximize:
		return htMaxButton
	case chromeClose:
		return htClose
	default:
		return htClient
	}
}

func (b *windowsWindowChromeBackend) postHover(part chromePart) {
	b.post(HoverEvent{Part: part})
}

func (b *windowsWindowChromeBackend) showSystemMenuAtWindowCenter() {
	b.mu.RLock()
	hwnd := b.hwnd
	b.mu.RUnlock()
	if hwnd == 0 {
		return
	}
	var rect winRect
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	b.showSystemMenu(packWinPoint(winPoint{
		X: (rect.Left + rect.Right) / 2,
		Y: rect.Top + (rect.Bottom-rect.Top)/4,
	}))
}

func (b *windowsWindowChromeBackend) showSystemMenu(lParam uintptr) {
	b.mu.RLock()
	hwnd := b.hwnd
	b.mu.RUnlock()
	if hwnd == 0 {
		return
	}
	menu, _, _ := procGetSystemMenu.Call(hwnd, 0)
	if menu == 0 {
		return
	}
	x, y := signedPackedPoint(lParam)
	procSetForegroundWindow.Call(hwnd)
	command, _, _ := procTrackPopupMenu.Call(
		menu,
		tpmRightButton|tpmReturnCmd,
		uintptr(x),
		uintptr(y),
		0,
		hwnd,
		0,
	)
	if command != 0 {
		procSendMessage.Call(hwnd, wmSysCommand, command, 0)
		return
	}
	procPostMessage.Call(hwnd, wmNull, 0, 0)
}

func (b *windowsWindowChromeBackend) attachDrop(hwnd uintptr) {
	b.mu.Lock()
	if hwnd == 0 || b.dropTarget != nil {
		b.mu.Unlock()
		return
	}
	b.dropHWND = hwnd
	b.mu.Unlock()
	attachWindowsDrop(b, hwnd)
}

func (b *windowsWindowChromeBackend) detachDrop() {
	b.mu.Lock()
	target := b.dropTarget
	hwnd := b.dropHWND
	initialized := b.oleInitialized
	b.dropTarget = nil
	b.dropHWND = 0
	b.oleInitialized = false
	b.mu.Unlock()
	detachWindowsDrop(target, hwnd, initialized)
}
