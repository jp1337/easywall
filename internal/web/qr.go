package web

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image/png"

	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"
)

// qrSize is the rendered edge length in pixels. Large enough to scan off a
// laptop screen at arm's length, small enough that the data URI stays a few
// kilobytes.
const qrSize = 256

// qrPNGDataURI renders text as a QR code, dark on white, as a data: URI.
//
// This file is the only place that knows about boombuler/barcode, and it is its
// own file at twenty lines for exactly that reason: it is the seam to the only
// dependency this release adds, and a seam with a name can be removed. A QR
// encoder is Reed–Solomon over GF(256) and is not our craft.
//
// A data: URI rather than a route, because the CSP already allows
// img-src 'self' data: and no rule has to be relaxed — and because a URL that
// serves an unconfirmed secret is a URL somebody can be induced to open.
//
// Dark on white in both themes, deliberately: an inverted QR code is rejected by
// a good share of scanners, so the dark theme puts it on a white plate rather
// than recolouring it. That is a defect only a screenshot in the dark theme
// shows.
func qrPNGDataURI(text string) (string, error) {
	code, err := qr.Encode(text, qr.M, qr.Auto)
	if err != nil {
		return "", fmt.Errorf("encode QR: %w", err)
	}
	scaled, err := barcode.Scale(code, qrSize, qrSize)
	if err != nil {
		return "", fmt.Errorf("scale QR: %w", err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, scaled); err != nil {
		return "", fmt.Errorf("encode QR PNG: %w", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
