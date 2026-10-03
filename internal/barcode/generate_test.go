package barcode

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
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

func TestCode128DataBounds(t *testing.T) {
	for _, tc := range []struct {
		name  string
		data  string
		valid bool
	}{
		{"empty", "", false},
		{"one", "A", true},
		{"eighty", strings.Repeat("A", 80), true},
		{"eighty-one", strings.Repeat("A", 81), false},
		{"non-ascii", "é", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := DefaultGenerateOptions()
			o.Symbology, o.Data = Code128, tc.data
			_, err := GeneratePlan(o)
			if tc.valid && err != nil {
				t.Fatal(err)
			}
			if !tc.valid && !errors.Is(err, ErrInvalidData) {
				t.Fatalf("expected ErrInvalidData, got %v", err)
			}
			if tc.name == "eighty-one" && !strings.Contains(err.Error(), "Code128 requires 1..80 ASCII characters") {
				t.Fatalf("domain length contract missing: %v", err)
			}
		})
	}
}

func TestCode128LongTextGeometry(t *testing.T) {
	for _, data := range []string{strings.Repeat("12", 40), strings.Repeat("A", 80)} {
		t.Run(data, func(t *testing.T) {
			o := DefaultGenerateOptions()
			o.Symbology, o.Data, o.ModuleWidth = Code128, data, 1
			p, err := GeneratePlan(o)
			if err != nil {
				t.Fatal(err)
			}
			textWidth := font.MeasureString(basicfont.Face7x13, data).Ceil()
			barWidth := p.code.Bounds().Dx()
			wantWidth := max(barWidth+20, textWidth+20)
			if p.Width != wantWidth {
				t.Fatalf("width=%d, want %d for complete text and margins", p.Width, wantWidth)
			}
			img, err := p.Render(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if img.Bounds().Dx() != p.Width || img.Bounds().Dy() != p.Height {
				t.Fatal("render bounds differ from admission plan")
			}
			// Compare every text pixel with a full, unclipped fixed-font rendering.
			want := image.NewGray(image.Rect(0, 0, wantWidth, 20))
			for i := range want.Pix {
				want.Pix[i] = 255
			}
			d := font.Drawer{Dst: want, Src: image.Black, Face: basicfont.Face7x13, Dot: fixed.P((wantWidth-textWidth)/2, 16)}
			d.DrawString(data)
			textStart := (10 + o.ModuleHeight) * img.Stride
			if !bytes.Equal(img.Pix[textStart:textStart+len(want.Pix)], want.Pix) {
				t.Fatal("human-readable text was clipped or misplaced")
			}
			barStart := (wantWidth - barWidth) / 2
			for x := 0; x < wantWidth; x++ {
				wantPixel := color.Gray{Y: 255}
				if x >= barStart && x < barStart+barWidth {
					wantPixel = color.GrayModel.Convert(p.code.At(x-barStart, 0)).(color.Gray)
				}
				if img.GrayAt(x, 10) != wantPixel {
					t.Fatalf("barcode is not centered at x=%d", x)
				}
			}
			o.DrawText = false
			without, err := GeneratePlan(o)
			if err != nil || without.Width != barWidth+20 {
				t.Fatalf("text-disabled geometry: %+v, %v", without, err)
			}
		})
	}
}

func TestGenerateCreatesParentDirectories(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(fmt.Sprintf("force=%t", force), func(t *testing.T) {
			dst := filepath.Join(t.TempDir(), "codes", "nested", "example.png")
			o := DefaultGenerateOptions()
			o.Data = "hello"
			r, err := GenerateFile(context.Background(), dst, o, force)
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(dst)
			if err != nil {
				t.Fatal(err)
			}
			img, err := png.Decode(bytes.NewReader(data))
			if err != nil || img.Bounds().Dx() != r.Output.Width || int64(len(data)) != r.Output.SizeBytes {
				t.Fatal("generated PNG does not match report", err)
			}
			entries, err := os.ReadDir(filepath.Dir(dst))
			if err != nil || len(entries) != 1 {
				t.Fatalf("temporary output left behind: %v, %v", entries, err)
			}
		})
	}
}

func TestGeneratedFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file permissions do not apply on Windows")
	}
	for _, tc := range []struct {
		name     string
		force    bool
		existing bool
	}{
		{"new", false, false},
		{"new-force", true, false},
		{"replace", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dst := filepath.Join(t.TempDir(), "code.png")
			if tc.existing {
				if err := os.WriteFile(dst, []byte("replace me"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			o := DefaultGenerateOptions()
			o.Data = "hello"
			if _, err := GenerateFile(context.Background(), dst, o, tc.force); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(dst)
			if err != nil {
				t.Fatal(err)
			}
			if got := info.Mode().Perm(); got != 0o644 {
				t.Fatalf("mode=%o, want 0644", got)
			}
		})
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
	afterConflict, err := os.ReadFile(dst)
	if err != nil || !bytes.Equal(data, afterConflict) {
		t.Fatal("no-clobber changed existing output", err)
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
	replacement, err := os.ReadFile(dst)
	if err != nil || bytes.Equal(data, replacement) {
		t.Fatal("force did not replace output", err)
	}
	if _, err := png.Decode(bytes.NewReader(replacement)); err != nil {
		t.Fatal("force committed an incomplete PNG", err)
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
	dst := filepath.Join(t.TempDir(), "codes", "nested", "code.png")
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
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		t.Fatal("winning output is not a complete PNG", err)
	}
	entries, err := os.ReadDir(filepath.Dir(dst))
	if err != nil || len(entries) != 1 {
		t.Fatalf("concurrent generation left partial files: %v, %v", entries, err)
	}
}

func TestGenerationCommitFailure(t *testing.T) {
	// With force, a directory destination makes the final rename fail after staging.
	for _, force := range []bool{false, true} {
		t.Run(fmt.Sprintf("force=%t", force), func(t *testing.T) {
			dir := t.TempDir()
			dst := filepath.Join(dir, "code.png")
			if err := os.Mkdir(dst, 0o755); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(dst, "existing")
			if err := os.WriteFile(marker, []byte("preserve"), 0o600); err != nil {
				t.Fatal(err)
			}
			o := DefaultGenerateOptions()
			o.Data = "hello"
			_, err := GenerateFile(context.Background(), dst, o, force)
			if err == nil || (!force && !errors.Is(err, ErrTargetExists)) {
				t.Fatalf("expected commit failure, got %v", err)
			}
			data, err := os.ReadFile(marker)
			if err != nil || string(data) != "preserve" {
				t.Fatal("failed commit changed existing destination", err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("failed commit left partial files: %v, %v", entries, err)
			}
		})
	}
}
