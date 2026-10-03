package barcode

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	bc "github.com/boombuler/barcode"
	"github.com/boombuler/barcode/code128"
	"github.com/boombuler/barcode/code39"
	"github.com/boombuler/barcode/ean"
	"github.com/boombuler/barcode/qr"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
)

type GenerateOptions struct {
	Symbology       Symbology
	Data            string
	ErrorCorrection string
	ModuleSize      int
	Border          int
	DrawText        bool
	ModuleWidth     int
	ModuleHeight    int
}

// DefaultGenerateOptions supplies defaults; explicit zero dimensions are invalid.
func DefaultGenerateOptions() GenerateOptions {
	return GenerateOptions{Symbology: QRCode, ErrorCorrection: "M", ModuleSize: 10, Border: 4, DrawText: true, ModuleWidth: 2, ModuleHeight: 80}
}

type Plan struct {
	Width        int
	Height       int
	WorkingBytes int64
	code         bc.Barcode
	opts         GenerateOptions
}

func GeneratePlan(opts GenerateOptions) (Plan, error) {
	s, err := ParseSymbology(string(opts.Symbology))
	if err != nil {
		return Plan{}, err
	}
	opts.Symbology = s
	if s == MicroQRCode || s == RMQRCode {
		return Plan{}, fmt.Errorf("%w: %s is decode-only", ErrInvalidSymbology, s)
	}
	if s == Code128 && (len(opts.Data) < 1 || len(opts.Data) > 80) {
		return Plan{}, fmt.Errorf("%w: Code128 requires 1..80 ASCII characters", ErrInvalidData)
	}
	if opts.Data == "" || !utf8.ValidString(opts.Data) || len(opts.Data) > 8192 {
		return Plan{}, fmt.Errorf("%w: content must be valid UTF-8 with 1..8192 bytes", ErrInvalidData)
	}
	var code bc.Barcode
	if s == QRCode {
		opts.ErrorCorrection = strings.ToUpper(strings.TrimSpace(opts.ErrorCorrection))
		levels := map[string]qr.ErrorCorrectionLevel{"L": qr.L, "M": qr.M, "Q": qr.Q, "H": qr.H}
		level, ok := levels[opts.ErrorCorrection]
		if !ok || opts.ModuleSize <= 0 || opts.ModuleSize > 1000000 || opts.Border < 0 || opts.Border > 1000000 {
			return Plan{}, fmt.Errorf("%w: QR requires L/M/Q/H, positive module-size and nonnegative border", ErrInvalidOptions)
		}
		code, err = qr.Encode(opts.Data, level, qr.Auto)
		opts.DrawText = false
	} else {
		if opts.ModuleWidth <= 0 || opts.ModuleWidth > 1000000 || opts.ModuleHeight <= 0 || opts.ModuleHeight > 1000000 {
			return Plan{}, fmt.Errorf("%w: module-width and module-height must be positive", ErrInvalidOptions)
		}
		switch s {
		case Code128:
			for _, r := range opts.Data {
				if r > 127 {
					return Plan{}, fmt.Errorf("%w: Code128 requires 1..80 ASCII characters", ErrInvalidData)
				}
			}
			code, err = code128.Encode(opts.Data)
		case Code39:
			code, err = code39.Encode(opts.Data, false, false)
		case EAN13, EAN8:
			n := 12
			if s == EAN8 {
				n = 7
			}
			if len(opts.Data) != n && len(opts.Data) != n+1 {
				return Plan{}, fmt.Errorf("%w: %s requires %d or %d digits", ErrInvalidData, s, n, n+1)
			}
			for _, r := range opts.Data {
				if r < '0' || r > '9' {
					return Plan{}, fmt.Errorf("%w: EAN requires decimal digits", ErrInvalidData)
				}
			}
			code, err = ean.Encode(opts.Data)
		}
	}
	if err != nil {
		return Plan{}, fmt.Errorf("%w: %v", ErrInvalidData, err)
	}
	w := int64(code.Bounds().Dx())
	var h int64
	if s == QRCode {
		w = (w + 2*int64(opts.Border)) * int64(opts.ModuleSize)
		h = w
	} else {
		// A fixed ten-module quiet zone on each side, independent of the library.
		w = (w + 20) * int64(opts.ModuleWidth)
		h = int64(opts.ModuleHeight) + 20
		if opts.DrawText {
			// Fixed 10 px text margins preserve every glyph even for compact codes.
			textWidth := int64(font.MeasureString(basicfont.Face7x13, code.Content()).Ceil())
			w = max(w, textWidth+20)
			h += 20
		}
	}
	// Plan arithmetic is bounded before converting to int or allocating pixels.
	if w > 2147483647 || h > 2147483647 || w > (1<<60)/h {
		return Plan{}, fmt.Errorf("%w: output dimensions overflow", ErrInvalidOptions)
	}
	return Plan{Width: int(w), Height: int(h), WorkingBytes: w*h*2 + 1<<20, code: code, opts: opts}, nil
}

type Output struct {
	Path      string `json:"path"`
	Format    string `json:"format"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	SizeBytes int64  `json:"size_bytes"`
}

type GenerateResult struct {
	SchemaVersion   string    `json:"schema_version"`
	Symbology       Symbology `json:"symbology"`
	Data            string    `json:"data"`
	EncodedText     string    `json:"encoded_text"`
	DrawText        bool      `json:"draw_text"`
	ErrorCorrection string    `json:"error_correction,omitempty"`
	ModuleSize      int       `json:"module_size,omitempty"`
	Border          *int      `json:"border,omitempty"`
	Output          Output    `json:"output"`
}

func GenerateFile(ctx context.Context, dst string, opts GenerateOptions, force bool) (GenerateResult, error) {
	p, err := GeneratePlan(opts)
	if err != nil {
		return GenerateResult{}, err
	}
	return p.WriteFile(ctx, dst, force)
}

// WriteFile commits a complete PNG. Without force, Link supplies atomic no-clobber.
func (p Plan) WriteFile(ctx context.Context, dst string, force bool) (GenerateResult, error) {
	if strings.ToLower(filepath.Ext(dst)) != ".png" {
		return GenerateResult{}, fmt.Errorf("%w: output must have .png extension", ErrInvalidOptions)
	}
	if err := ctx.Err(); err != nil {
		return GenerateResult{}, err
	}
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return GenerateResult{}, err
	}
	if _, err := os.Lstat(dst); err == nil && !force {
		return GenerateResult{}, ErrTargetExists
	} else if err != nil && !os.IsNotExist(err) {
		return GenerateResult{}, err
	}
	img, err := p.Render(ctx)
	if err != nil {
		return GenerateResult{}, err
	}
	f, err := os.CreateTemp(dir, ".itb-barcode-*.png")
	if err != nil {
		return GenerateResult{}, err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	defer func() { _ = f.Close() }()
	if err = png.Encode(f, img); err != nil {
		return GenerateResult{}, err
	}
	if err = f.Sync(); err != nil {
		return GenerateResult{}, err
	}
	info, err := f.Stat()
	if err != nil {
		return GenerateResult{}, err
	}
	if err = f.Close(); err != nil {
		return GenerateResult{}, err
	}
	if err = ctx.Err(); err != nil {
		return GenerateResult{}, err
	}
	if force {
		err = os.Rename(f.Name(), dst)
	} else {
		err = os.Link(f.Name(), dst)
	}
	if os.IsExist(err) {
		return GenerateResult{}, ErrTargetExists
	}
	if err != nil {
		return GenerateResult{}, err
	}
	r := GenerateResult{SchemaVersion: "itb.barcode.generate.v1", Symbology: p.opts.Symbology, Data: p.opts.Data, EncodedText: p.code.Content(), DrawText: p.opts.DrawText, Output: Output{Path: dst, Format: "png", Width: p.Width, Height: p.Height, SizeBytes: info.Size()}}
	if p.opts.Symbology == QRCode {
		r.ErrorCorrection = p.opts.ErrorCorrection
		r.ModuleSize = p.opts.ModuleSize
		border := p.opts.Border
		r.Border = &border
	}
	return r, nil
}

// Render returns deterministic opaque black/white pixels, without system fonts.
func (p Plan) Render(ctx context.Context) (*image.Gray, error) { return render(ctx, p) }
