//go:build !windows

package windowing

type portableWindowChromeBackend struct{}

func newWindowChromeBackend(_ func(any)) windowChromeBackend {
	return portableWindowChromeBackend{}
}

func (portableWindowChromeBackend) handlePlatformEvent(any) bool {
	return false
}

func (portableWindowChromeBackend) setCaptionRegions(chromeHitRegions) {}

func (portableWindowChromeBackend) close() {}
