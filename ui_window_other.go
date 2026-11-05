//go:build !windows

package main

type portableWindowChromeBackend struct{}

func newWindowChromeBackend(_ *appUI) windowChromeBackend {
	return portableWindowChromeBackend{}
}

func (portableWindowChromeBackend) handlePlatformEvent(any) bool {
	return false
}

func (portableWindowChromeBackend) setCaptionRegions(chromeHitRegions) {}

func (portableWindowChromeBackend) close() {}
