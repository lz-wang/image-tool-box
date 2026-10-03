package barcode

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeDecoder func(context.Context, *image.Gray, []Symbology) ([]RawCode, error)

func (f fakeDecoder) Decode(ctx context.Context, img *image.Gray, s []Symbology) ([]RawCode, error) {
	return f(ctx, img, s)
}

func TestDecodeDomain(t *testing.T) {
	img := image.NewNRGBA(image.Rect(3, 5, 23, 25))
	img.SetNRGBA(3, 5, color.NRGBA{R: 255, A: 128})
	points := []Point{{1, 2}, {10, 2}, {10, 12}, {1, 12}}
	decoder := fakeDecoder(func(_ context.Context, gray *image.Gray, formats []Symbology) ([]RawCode, error) {
		if gray.Bounds().Min != (image.Point{}) || gray.GrayAt(1, 1).Y != 255 || gray.GrayAt(0, 0).Y < 160 {
			t.Fatal("logical bounds/alpha background")
		}
		if len(formats) != 7 {
			t.Fatal(formats)
		}
		var result []RawCode
		for _, name := range []string{"QRCodeModel2", "MicroQRCode", "RMQRCode", "Code128", "Code39", "EAN13", "EAN8", "QRCodeModel2"} {
			result = append(result, RawCode{Format: name, Text: "same", Points: points})
		}
		return result, nil
	})
	r, err := decodeImage(context.Background(), img, allSymbologies(), decoder)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Codes) != 8 || r.Codes[1].Symbology != MicroQRCode || r.Codes[2].Symbology != RMQRCode || r.Codes[0].Points[0] != (Point{1, 2}) {
		t.Fatalf("mapping: %+v", r)
	}
	decoder = func(context.Context, *image.Gray, []Symbology) ([]RawCode, error) { return nil, nil }
	r, err = decodeImage(context.Background(), img, allSymbologies(), decoder)
	if err != nil || r.Codes == nil || len(r.Codes) != 0 {
		t.Fatal("empty must be successful nonnil slice")
	}
}

func TestDecodeErrors(t *testing.T) {
	if _, err := normalizeSymbologies([]Symbology{"aztec"}); !errors.Is(err, ErrInvalidSymbology) {
		t.Fatal(err)
	}
	s, err := normalizeSymbologies([]Symbology{" QR ", QRCode})
	if err != nil || len(s) != 1 {
		t.Fatal(s, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := DecodeFile(ctx, "missing.png", DecodeOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := DecodeFile(context.Background(), filepath.Join(t.TempDir(), "missing.png"), DecodeOptions{}); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	img := image.NewGray(image.Rect(0, 0, 20, 20))
	for _, raw := range []RawCode{{Format: "unknown"}, {Format: "QRCodeModel2"}, {Format: "QRCodeModel2", Points: []Point{{-1, 0}, {0, 0}, {0, 0}, {0, 0}}}} {
		decoder := fakeDecoder(func(context.Context, *image.Gray, []Symbology) ([]RawCode, error) { return []RawCode{raw}, nil })
		if _, err := decodeImage(context.Background(), img, allSymbologies(), decoder); !errors.Is(err, ErrDecoderFailure) {
			t.Fatal(err)
		}
	}
	for _, size := range []image.Point{{0, 1}, {32769, 1}, {10000, 10000}} {
		if _, err := DecodeWorkingBytes(size.X, size.Y); err == nil {
			t.Fatal(size)
		}
	}
}

func TestPrivateProtocol(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 4, 4))
	img.Pix[5] = 42
	sub := img.SubImage(image.Rect(1, 1, 3, 3)).(*image.Gray)
	var frame bytes.Buffer
	if err := writeFrame(&frame, sub, []Symbology{QRCode, EAN8}); err != nil {
		t.Fatal(err)
	}
	data := frame.Bytes()
	if len(data) != 25 || string(data[:4]) != "ITBZ" || data[4] != 1 || binary.BigEndian.Uint32(data[5:]) != 2 || binary.BigEndian.Uint32(data[17:]) != 65 || data[21] != 42 {
		t.Fatalf("frame: %v", data)
	}
	for _, value := range []string{"", "{", "{}", "{\"version\":2,\"codes\":[]}", "{\"version\":1,\"codes\":null}", "{\"version\":1,\"codes\":[]}{}", "{\"version\":1,\"codes\":[],\"extra\":true}"} {
		if _, err := parseResponse(strings.NewReader(value)); !errors.Is(err, ErrDecoderFailure) {
			t.Fatalf("%q: %v", value, err)
		}
	}
	if codes, err := parseResponse(strings.NewReader("{\"version\":1,\"codes\":[]}\n")); err != nil || len(codes) != 0 {
		t.Fatal(err)
	}
}
