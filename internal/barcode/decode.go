package barcode

import (
	"context"
	"fmt"
	"image"

	"imagetoolbox/internal/imageio"
)

type DecodeOptions struct{ Symbologies []Symbology }
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}
type Code struct {
	Text      string    `json:"text"`
	Symbology Symbology `json:"symbology"`
	Points    []Point   `json:"points"`
}
type Input struct {
	Path   string `json:"path"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}
type DecodeResult struct {
	SchemaVersion string `json:"schema_version"`
	Input         Input  `json:"input"`
	Codes         []Code `json:"codes"`
}

// DecodeWorkingBytes includes decoded/oriented NRGBA, Gray8, protocol and reader
// scratch storage. HTTP must admit this before OpenStatic or Gray allocation.
func DecodeWorkingBytes(width, height int) (int64, error) {
	if width <= 0 || height <= 0 || width > 32768 || height > 32768 || int64(width)*int64(height) > maxDecodePixels {
		return 0, fmt.Errorf("%w: decode supports at most 32768 per dimension and 64 Mi pixels", ErrInvalidImage)
	}
	return int64(width)*int64(height)*16 + 8<<20, nil
}

func DecodeFile(ctx context.Context, path string, opts DecodeOptions) (DecodeResult, error) {
	return decodeFile(ctx, path, opts, nativeDecoder{})
}

func decodeFile(ctx context.Context, path string, opts DecodeOptions, decoder Decoder) (DecodeResult, error) {
	formats, err := normalizeSymbologies(opts.Symbologies)
	if err != nil {
		return DecodeResult{}, err
	}
	if err = ctx.Err(); err != nil {
		return DecodeResult{}, err
	}
	info, err := imageio.Probe(path)
	if err != nil {
		return DecodeResult{}, fmt.Errorf("%w: %w", ErrInvalidImage, err)
	}
	if _, err = DecodeWorkingBytes(info.Width, info.Height); err != nil {
		return DecodeResult{}, err
	}
	img, err := imageio.OpenStatic(path)
	if err != nil {
		return DecodeResult{}, fmt.Errorf("%w: %w", ErrInvalidImage, err)
	}
	result, err := decodeImage(ctx, img, formats, decoder)
	result.Input.Path = path
	return result, err
}

func decodeImage(ctx context.Context, img image.Image, formats []Symbology, decoder Decoder) (DecodeResult, error) {
	b := img.Bounds()
	if _, err := DecodeWorkingBytes(b.Dx(), b.Dy()); err != nil {
		return DecodeResult{}, err
	}
	gray := image.NewGray(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		if err := ctx.Err(); err != nil {
			return DecodeResult{}, err
		}
		for x := 0; x < b.Dx(); x++ {
			r, g, bl, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			white := uint32(65535) - a
			r += white
			g += white
			bl += white
			gray.Pix[y*gray.Stride+x] = uint8(((299*r + 587*g + 114*bl + 500) / 1000) >> 8)
		}
	}
	raw, err := decoder.Decode(ctx, gray, formats)
	if err != nil {
		return DecodeResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return DecodeResult{}, err
	}
	result := DecodeResult{SchemaVersion: "itb.barcode.decode.v1", Input: Input{Width: b.Dx(), Height: b.Dy()}, Codes: make([]Code, 0, len(raw))}
	allowed := map[Symbology]bool{}
	for _, s := range formats {
		allowed[s] = true
	}
	for _, value := range raw {
		code, err := normalizeCode(value)
		if err != nil {
			return DecodeResult{}, err
		}
		if !allowed[code.Symbology] {
			return DecodeResult{}, fmt.Errorf("%w: unrequested symbology", ErrDecoderFailure)
		}
		for _, point := range code.Points {
			if point.X < 0 || point.Y < 0 || point.X > b.Dx() || point.Y > b.Dy() {
				return DecodeResult{}, fmt.Errorf("%w: position outside image", ErrDecoderFailure)
			}
		}
		result.Codes = append(result.Codes, code)
	}
	return result, nil
}
