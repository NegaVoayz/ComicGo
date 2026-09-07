package imgutil

import (
	"bytes"
	"image"
	"image/jpeg"
	"testing"
)

func jpegSize(t *testing.T, b []byte) (int, int) {
	t.Helper()
	c, err := jpeg.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return c.Width, c.Height
}

func TestThumbJPEGFitsBound(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 800, 600))
	pngBytes, err := EncodePNG(src)
	if err != nil {
		t.Fatal(err)
	}
	out, err := ThumbJPEG(pngBytes, 320, 80)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(out, []byte("\xff\xd8\xff")) {
		t.Error("output is not a JPEG")
	}
	w, h := jpegSize(t, out)
	if w != 320 || h != 240 {
		t.Errorf("thumb = %dx%d, want 320x240 (bound-preserving scale of 800x600)", w, h)
	}
}

func TestThumbUnderBoundStillEncodes(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 100, 100))
	b, _ := EncodePNG(src)
	out, err := ThumbJPEG(b, 320, 80)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Error("empty output")
	}
}

func TestSniffType(t *testing.T) {
	if got := SniffType([]byte{0xff, 0xd8, 0xff, 0xe0}); got != "image/jpeg" {
		t.Errorf("jpeg sniff = %q", got)
	}
	if got := SniffType([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}); got != "image/png" {
		t.Errorf("png sniff = %q", got)
	}
	if got := SniffType([]byte{0, 1}); got != "application/octet-stream" {
		t.Errorf("unknown sniff = %q", got)
	}
}

func TestDecodeSize(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 640, 480))
	b, _ := EncodePNG(src)
	w, h, err := DecodeSize(b)
	if err != nil {
		t.Fatal(err)
	}
	if w != 640 || h != 480 {
		t.Errorf("DecodeSize = %dx%d", w, h)
	}
}
