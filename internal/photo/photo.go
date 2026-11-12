// Package photo validates the cover photos users upload.
package photo

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// MaxBytes is the largest decoded photo accepted.
const MaxBytes = 10 << 20

var (
	// ErrUnsupported is returned for anything that is not a JPEG, PNG or WebP image.
	ErrUnsupported = errors.New("a photo of the comic is required (JPEG, PNG or WebP)")
	// ErrTooLarge is returned when the decoded photo exceeds MaxBytes.
	ErrTooLarge = errors.New("photo is too large (10 MB limit)")
	// ErrInvalid is returned when the data URL cannot be decoded.
	ErrInvalid = errors.New("photo data is not valid base64")
)

// Photo is a validated image ready to be stored.
type Photo struct {
	Data        []byte
	Extension   string
	ContentType string
}

var extensions = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
}

// ParseDataURL decodes a base64 data URL such as the one a browser produces
// from a canvas. The image type is decided by sniffing the bytes, so a
// mislabelled file is rejected.
func ParseDataURL(dataURL string) (Photo, error) {
	header, payload, ok := strings.Cut(dataURL, ",")
	if !ok || !strings.HasPrefix(header, "data:") || !strings.HasSuffix(header, ";base64") {
		return Photo{}, ErrUnsupported
	}
	declared := strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64")
	if _, ok := extensions[declared]; !ok {
		return Photo{}, ErrUnsupported
	}

	// Each base64 character carries six bits, so this bounds the decoded size
	// before any large allocation happens.
	if len(payload)/4*3 > MaxBytes {
		return Photo{}, ErrTooLarge
	}
	data, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(payload), ""))
	if err != nil {
		return Photo{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if len(data) == 0 {
		return Photo{}, ErrUnsupported
	}
	if len(data) > MaxBytes {
		return Photo{}, ErrTooLarge
	}

	sniffed := http.DetectContentType(data[:min(len(data), 512)])
	ext, ok := extensions[sniffed]
	if !ok {
		return Photo{}, ErrUnsupported
	}
	return Photo{Data: data, Extension: ext, ContentType: sniffed}, nil
}
