package watermark

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestAddImageWatermark(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input.png")
	logo := filepath.Join(dir, "logo.png")
	output := filepath.Join(dir, "output.png")

	base := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	fill(base, color.NRGBA{255, 255, 255, 255})
	writePNG(t, input, base)

	wm := image.NewNRGBA(image.Rect(0, 0, 20, 20))
	fill(wm, color.NRGBA{255, 0, 0, 255})
	writePNG(t, logo, wm)

	got, err := AddImageWatermark(input, output, &ImageOptions{
		ImagePath:  logo,
		Position:   BottomRight,
		ScaleRatio: float64Ptr(0.2),
		Opacity:    float64Ptr(1),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r, g, b, _ := got.At(95, 95).RGBA()
	if r == 0 || g != 0 || b != 0 {
		t.Fatal("expected bottom-right watermark pixel to be red")
	}
}

func fill(img *image.NRGBA, c color.NRGBA) {
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
}

func writePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create png: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
}

func float64Ptr(v float64) *float64 {
	return &v
}
