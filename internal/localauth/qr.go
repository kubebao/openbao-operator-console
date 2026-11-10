package localauth

import (
	"strconv"
	"strings"

	"rsc.io/qr"
)

// qrQuietZone is the blank margin, in modules, that QR scanners require.
const qrQuietZone = 4

// QRCode is a QR code as SVG path data, so the browser can draw it without images,
// inline markup, or a content security policy exception.
type QRCode struct {
	// Size is the width and height in modules, including the quiet zone.
	Size int
	// Path draws one unit square per dark module.
	Path string
}

// NewQRCode encodes text with medium error correction.
func NewQRCode(text string) (QRCode, error) {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return QRCode{}, err
	}
	var path strings.Builder
	for y := range code.Size {
		for x := 0; x < code.Size; x++ {
			if !code.Black(x, y) {
				continue
			}
			// Merge horizontal runs of dark modules into one rectangle.
			start := x
			for x+1 < code.Size && code.Black(x+1, y) {
				x++
			}
			path.WriteString("M" + strconv.Itoa(start+qrQuietZone) + " " + strconv.Itoa(y+qrQuietZone))
			path.WriteString("h" + strconv.Itoa(x-start+1) + "v1h-" + strconv.Itoa(x-start+1) + "z")
		}
	}
	return QRCode{Size: code.Size + 2*qrQuietZone, Path: path.String()}, nil
}
