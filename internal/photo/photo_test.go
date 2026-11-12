package photo

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

func dataURL(mime string, data []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

func solidImage(t *testing.T) *image.RGBA {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for x := range 4 {
		for y := range 4 {
			img.Set(x, y, color.RGBA{R: 200, G: 30, B: 30, A: 255})
		}
	}
	return img
}

func TestParseDataURLAcceptsSupportedImages(t *testing.T) {
	var jpgBuf, pngBuf bytes.Buffer
	if err := jpeg.Encode(&jpgBuf, solidImage(t), nil); err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(&pngBuf, solidImage(t)); err != nil {
		t.Fatal(err)
	}
	webp := append([]byte("RIFF\x10\x00\x00\x00WEBPVP8 "), make([]byte, 16)...)

	tests := []struct {
		name string
		url  string
		ext  string
	}{
		{"jpeg", dataURL("image/jpeg", jpgBuf.Bytes()), "jpg"},
		{"png", dataURL("image/png", pngBuf.Bytes()), "png"},
		{"webp", dataURL("image/webp", webp), "webp"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDataURL(tt.url)
			if err != nil {
				t.Fatalf("ParseDataURL() error = %v", err)
			}
			if got.Extension != tt.ext {
				t.Errorf("Extension = %q, want %q", got.Extension, tt.ext)
			}
			if len(got.Data) == 0 {
				t.Error("Data is empty")
			}
		})
	}
}

func TestParseDataURLRejectsWhatItCannotStore(t *testing.T) {
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, solidImage(t)); err != nil {
		t.Fatal(err)
	}

	tests := map[string]struct {
		url     string
		wantErr error
	}{
		"not a data url":        {"https://example.test/a.jpg", ErrUnsupported},
		"plain text mime":       {dataURL("text/plain", []byte("hi")), ErrUnsupported},
		"image labelled as gif": {dataURL("image/gif", pngBuf.Bytes()), ErrUnsupported},
		"png labelled as jpeg":  {dataURL("image/jpeg", []byte("not an image at all")), ErrUnsupported},
		"empty payload":         {"data:image/jpeg;base64,", ErrUnsupported},
		"bad base64":            {"data:image/png;base64,@@@@", ErrInvalid},
		"too large":             {"data:image/jpeg;base64," + strings.Repeat("A", MaxBytes/3*4+8), ErrTooLarge},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := ParseDataURL(tt.url)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ParseDataURL() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
