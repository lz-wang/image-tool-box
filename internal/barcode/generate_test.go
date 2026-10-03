package barcode

import (
	"bytes"
	"context"
	"errors"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestGeneratePlans(t *testing.T) {
	for _, tc := range []struct {
		s             Symbology
		data, encoded string
		width         int
	}{
		{QRCode, "hello", "hello", 290}, {Code128, "ABC123", "ABC123", 242},
		{Code39, "ABC-123", "ABC-123", 272}, {EAN13, "690123456789", "6901234567892", 230},
		{EAN13, "6901234567892", "6901234567892", 230}, {EAN8, "9638507", "96385074", 174},
		{EAN8, "96385074", "96385074", 174},
	} {
		t.Run(string(tc.s)+tc.data, func(t *testing.T) {
			o := DefaultGenerateOptions()
			o.Symbology = tc.s
			o.Data = tc.data
			p, err := GeneratePlan(o)
			if err != nil {
				t.Fatal(err)
			}
			if p.code.Content() != tc.encoded || p.Width != tc.width {
				t.Fatalf("plan=%+v encoded=%s", p, p.code.Content())
			}
			img, err := p.Render(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if img.Bounds().Dx() != p.Width || img.Bounds().Dy() != p.Height {
				t.Fatal("bounds differ from plan")
			}
			if img.GrayAt(0, 0) != (color.Gray{Y: 255}) {
				t.Fatal("quiet zone must be white")
			}
		})
	}
}

func TestQRGeometry(t *testing.T) {
	for _, level := range []string{"L", "M", "Q", "H"} {
		for _, border := range []int{0, 4, 7} {
			t.Run(level+string(rune('0'+border)), func(t *testing.T) {
				o := DefaultGenerateOptions()
				o.Data = "hello"
				o.ErrorCorrection = level
				o.ModuleSize = 3
				o.Border = border
				p, err := GeneratePlan(o)
				if err != nil {
					t.Fatal(err)
				}
				if p.Width != (21+2*border)*3 || p.Height != p.Width {
					t.Fatalf("dimensions: %+v", p)
				}
			})
		}
	}
}

func TestGenerationRejectsInvalid(t *testing.T) {
	for _, tc := range []struct {
		name   string
		modify func(*GenerateOptions)
	}{
		{"empty", func(o *GenerateOptions) { o.Data = "" }},
		{"oversized", func(o *GenerateOptions) { o.Data = strings.Repeat("x", 9000) }},
		{"micro", func(o *GenerateOptions) { o.Symbology = MicroQRCode }},
		{"rmqr", func(o *GenerateOptions) { o.Symbology = RMQRCode }},
		{"unknown", func(o *GenerateOptions) { o.Symbology = "aztec" }},
		{"zero-module", func(o *GenerateOptions) { o.ModuleSize = 0 }},
		{"negative-border", func(o *GenerateOptions) { o.Border = -1 }},
		{"bad-ecc", func(o *GenerateOptions) { o.ErrorCorrection = "Z" }},
		{"ean-check", func(o *GenerateOptions) { o.Symbology = EAN13; o.Data = "6901234567891" }},
		{"ean-length", func(o *GenerateOptions) { o.Symbology = EAN8; o.Data = "690123456789" }},
		{"ean-letter", func(o *GenerateOptions) { o.Symbology = EAN8; o.Data = "963850x" }},
		{"code39", func(o *GenerateOptions) { o.Symbology = Code39; o.Data = "lowercase" }},
		{"code128", func(o *GenerateOptions) { o.Symbology = Code128; o.Data = "中文" }},
		{"zero-width", func(o *GenerateOptions) { o.Symbology = Code128; o.ModuleWidth = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := DefaultGenerateOptions()
			o.Data = "hello"
			tc.modify(&o)
			if _, err := GeneratePlan(o); err == nil {
				t.Fatal("accepted invalid options")
			}
		})
	}
}

func TestLinearTextAndDimensions(t *testing.T) {
	o := DefaultGenerateOptions()
	o.Symbology = Code39
	o.Data = "ABC"
	o.ModuleWidth = 3
	o.ModuleHeight = 60
	p, err := GeneratePlan(o)
	if err != nil {
		t.Fatal(err)
	}
	withText, _ := p.Render(context.Background())
	o.DrawText = false
	q, err := GeneratePlan(o)
	if err != nil {
		t.Fatal(err)
	}
	withoutText, _ := q.Render(context.Background())
	if p.Height-q.Height != 20 || p.Width != q.Width || q.Height != 80 {
		t.Fatal("text geometry")
	}
	// Code39 without checksum has (n+2)*12 + n+1 modules.
	if p.Width != (5*12+4+20)*3 {
		t.Fatal("checksum must be disabled")
	}
	if bytes.Equal(withText.Pix[withoutText.Stride*70:withoutText.Stride*80], withoutText.Pix[withoutText.Stride*70:]) {
		t.Fatal("text was not drawn")
	}
}

func TestAtomicGeneration(t *testing.T) {
	ctx := context.Background()
	o := DefaultGenerateOptions()
	o.Data = "hello"
	dir := t.TempDir()
	dst := filepath.Join(dir, "code.png")
	r, err := GenerateFile(ctx, dst, o, false)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(dst)
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil || img.Bounds().Dx() != r.Output.Width {
		t.Fatal("invalid PNG report")
	}
	if _, err = GenerateFile(ctx, dst, o, false); !errors.Is(err, ErrTargetExists) {
		t.Fatal(err)
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = GenerateFile(cancelCtx, dst, o, true); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(dst)
	if !bytes.Equal(data, after) {
		t.Fatal("failed generation replaced existing output")
	}
	o.Data = "replacement"
	if _, err = GenerateFile(ctx, dst, o, true); err != nil {
		t.Fatal(err)
	}
	if _, err = GenerateFile(ctx, filepath.Join(dir, "bad.jpg"), o, false); err == nil {
		t.Fatal("accepted non-PNG")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("partial files: %v", entries)
	}
}

func TestConcurrentNoClobber(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "code.png")
	o := DefaultGenerateOptions()
	o.Data = "race"
	results := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Go(func() { _, err := GenerateFile(context.Background(), dst, o, false); results <- err })
	}
	wg.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else if !errors.Is(err, ErrTargetExists) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("winners=%d", winners)
	}
}
