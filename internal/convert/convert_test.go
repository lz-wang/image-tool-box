package convert

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"imagetoolbox/internal/imageio"
)

func TestConvertPNGToJPEGWithBackground(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input.png")
	output := filepath.Join(dir, "output.jpg")

	img := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	img.Set(0, 0, color.NRGBA{0, 0, 0, 0})
	img.Set(5, 5, color.NRGBA{255, 0, 0, 255})

	writePNG(t, input, img)

	if err := ConvertFile(input, output, Options{
		To:         "jpg",
		Quality:    80,
		Background: "#00FF00",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out, err := imageio.DetectFormat(output)
	if err != nil {
		t.Fatalf("detect output format: %v", err)
	}
	if out != imageio.FormatJPEG {
		t.Fatalf("got %s, want jpeg", out)
	}
}

func TestConvertPNGToWEBP(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input.png")
	output := filepath.Join(dir, "output.webp")

	img := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	img.Set(2, 2, color.NRGBA{255, 0, 0, 255})
	writePNG(t, input, img)

	if err := ConvertFile(input, output, Options{
		To:      "webp",
		Quality: 80,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out, err := imageio.DetectFormat(output)
	if err != nil {
		t.Fatalf("detect output format: %v", err)
	}
	if out != imageio.FormatWEBP {
		t.Fatalf("got %s, want webp", out)
	}
}

func TestDefaultOutputPath(t *testing.T) {
	got, err := DefaultOutputPath("/tmp/a.png", "jpg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/tmp/a_converted.jpeg" {
		t.Fatalf("got %s", got)
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
