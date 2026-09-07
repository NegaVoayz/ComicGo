// Package imgutil decodes cover/page images and produces small JPEG
// thumbnails. Standard formats (jpeg/png/gif) register themselves; webp and
// bmp are sniffed by magic bytes because golang.org/x/image codecs do not
// auto-register.
package imgutil

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"

	_ "image/gif"

	"golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

// image.DecodeConfig needs codecs registered; the blank imports above register
// gif/jpeg/png. webp/bmp are handled explicitly in Decode/DecodeSize.

// MaxPixels bounds decode size to avoid pathological memory use from a single
// enormous page (roughly a 100 MP image).
const MaxPixels = 100_000_000

// Decode decodes any supported image format from raw bytes.
func Decode(b []byte) (image.Image, string, error) {
	if len(b) == 0 {
		return nil, "", fmt.Errorf("empty image data")
	}
	// webp: RIFF....WEBP
	if len(b) > 12 && bytes.Equal(b[:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")) {
		img, err := webp.Decode(bytes.NewReader(b))
		if err != nil {
			return nil, "webp", err
		}
		return img, "webp", nil
	}
	// bmp: "BM"
	if bytes.Equal(b[:2], []byte("BM")) {
		img, err := bmp.Decode(bytes.NewReader(b))
		if err != nil {
			return nil, "bmp", err
		}
		return img, "bmp", nil
	}
	img, format, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, "", err
	}
	return img, format, nil
}

// ThumbJPEG downscales the image so its longer edge is <= maxDim and returns
// JPEG bytes at the given quality. Dimensions are checked up front so a
// pathological image is rejected before a full (potentially huge) decode.
func ThumbJPEG(b []byte, maxDim, quality int) ([]byte, error) {
	w, h, err := DecodeSize(b)
	if err != nil {
		return nil, err
	}
	if w*h > MaxPixels {
		return nil, fmt.Errorf("image too large to thumbnail (%dx%d)", w, h)
	}
	img, _, err := Decode(b)
	if err != nil {
		return nil, err
	}
	thumb := ScaleToFit(img, maxDim)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, thumb, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// DecodeSize returns the pixel dimensions of an image without a full decode.
func DecodeSize(b []byte) (w, h int, err error) {
	if len(b) > 12 && bytes.Equal(b[:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")) {
		c, err := webp.DecodeConfig(bytes.NewReader(b))
		return c.Width, c.Height, err
	}
	if len(b) >= 2 && bytes.Equal(b[:2], []byte("BM")) {
		c, err := bmp.DecodeConfig(bytes.NewReader(b))
		return c.Width, c.Height, err
	}
	c, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return 0, 0, err
	}
	return c.Width, c.Height, nil
}

// ScaleToFit returns a copy of img scaled so its longer edge is <= maxDim.
// Images already within the bound are returned unchanged.
func ScaleToFit(img image.Image, maxDim int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxDim && h <= maxDim {
		return img
	}
	scale := float64(maxDim) / float64(max(w, h))
	nw, nh := max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	return dst
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// SniffType reports a MIME-ish label for bytes by magic number. Used when
// serving page bytes that are streamed through unchanged.
func SniffType(b []byte) string {
	if len(b) == 0 {
		return "application/octet-stream"
	}
	if bytes.Equal(b[:2], []byte("BM")) {
		return "image/bmp"
	}
	if len(b) > 12 && bytes.Equal(b[:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")) {
		return "image/webp"
	}
	switch {
	case bytes.HasPrefix(b, []byte("\xff\xd8\xff")):
		return "image/jpeg"
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case bytes.HasPrefix(b, []byte("GIF8")):
		return "image/gif"
	}
	return "application/octet-stream"
}

// EncodePNG is used only by tests to fabricate fixtures.
func EncodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	err := png.Encode(&buf, img)
	return buf.Bytes(), err
}
