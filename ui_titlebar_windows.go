//go:build windows

package main

import (
	"image/color"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	dwmwaUseImmersiveDarkMode = 20
	dwmwaBorderColor          = 34
	dwmwaCaptionColor         = 35
	dwmwaTextColor            = 36
)

var (
	dwmapi                    = windows.NewLazySystemDLL("dwmapi.dll")
	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
)

func applyDarkTitleBar(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	setDWMAttribute(hwnd, dwmwaUseImmersiveDarkMode, 1)
	// Only Windows 11 recognizes these color attributes. Older systems return
	// an error, which is harmless because immersive dark mode still applies.
	setDWMAttribute(hwnd, dwmwaCaptionColor, colorRef(colBG))
	setDWMAttribute(hwnd, dwmwaTextColor, colorRef(colText))
	setDWMAttribute(hwnd, dwmwaBorderColor, colorRef(colLine))
}

func setDWMAttribute(hwnd uintptr, attribute uint32, value uint32) {
	procDwmSetWindowAttribute.Call(
		hwnd,
		uintptr(attribute),
		uintptr(unsafe.Pointer(&value)),
		unsafe.Sizeof(value),
	)
}

func colorRef(c color.NRGBA) uint32 {
	return uint32(c.R) | uint32(c.G)<<8 | uint32(c.B)<<16
}
