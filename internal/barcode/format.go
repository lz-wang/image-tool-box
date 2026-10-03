// Package barcode owns generation and decoding semantics for the supported codes.
package barcode

import (
	"errors"
	"fmt"
	"strings"
)

type Symbology string

const (
	QRCode      Symbology = "qr"
	MicroQRCode Symbology = "micro-qr"
	RMQRCode    Symbology = "rmqr"
	Code128     Symbology = "code128"
	Code39      Symbology = "code39"
	EAN13       Symbology = "ean13"
	EAN8        Symbology = "ean8"
)

var (
	ErrInvalidSymbology   = errors.New("invalid symbology")
	ErrInvalidData        = errors.New("invalid barcode data")
	ErrInvalidOptions     = errors.New("invalid barcode options")
	ErrInvalidImage       = errors.New("invalid barcode image")
	ErrTargetExists       = errors.New("barcode destination exists")
	ErrDecoderUnavailable = errors.New("barcode decoder unavailable")
	ErrDecoderFailure     = errors.New("barcode decoder failed")
)

func ParseSymbology(value string) (Symbology, error) {
	s := Symbology(strings.ToLower(strings.TrimSpace(value)))
	switch s {
	case QRCode, MicroQRCode, RMQRCode, Code128, Code39, EAN13, EAN8:
		return s, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidSymbology, value)
	}
}
