package resize

import (
	"image"
	"image/color"
	"testing"
)

func TestApplyPercent(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 200, 100))
	got, err := Apply(img, Options{Percent: "50%"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Bounds().Dx() != 100 || got.Bounds().Dy() != 50 {
		t.Fatalf("got %dx%d, want 100x50", got.Bounds().Dx(), got.Bounds().Dy())
	}
}

func TestApplyWidthOnly(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 200, 100))
	got, err := Apply(img, Options{Width: 50})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Bounds().Dx() != 50 || got.Bounds().Dy() != 25 {
		t.Fatalf("got %dx%d, want 50x25", got.Bounds().Dx(), got.Bounds().Dy())
	}
}

func TestApplyStretch(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 200, 100))
	got, err := Apply(img, Options{Width: 50, Height: 50, Mode: ModeStretch})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Bounds().Dx() != 50 || got.Bounds().Dy() != 50 {
		t.Fatalf("got %dx%d, want 50x50", got.Bounds().Dx(), got.Bounds().Dy())
	}
}

func TestApplyFillUsesAnchor(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 200, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.NRGBA{255, 0, 0, 255})
		}
		for x := 100; x < 200; x++ {
			img.Set(x, y, color.NRGBA{0, 0, 255, 255})
		}
	}

	left, err := Apply(img, Options{Width: 50, Height: 50, Mode: ModeFill, Anchor: "left"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	right, err := Apply(img, Options{Width: 50, Height: 50, Mode: ModeFill, Anchor: "right"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lr, _, _, _ := left.At(5, 25).RGBA()
	rr, _, bb, _ := right.At(45, 25).RGBA()
	if lr == 0 {
		t.Fatal("expected left-anchored image to keep red area")
	}
	if rr != 0 || bb == 0 {
		t.Fatal("expected right-anchored image to keep blue area")
	}
}

func TestValidateOptions(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	_, err := Apply(img, Options{Percent: "50%", Width: 50})
	if err == nil {
		t.Fatal("expected conflict error")
	}

	_, err = Apply(img, Options{Mode: ModeFill, Width: 50})
	if err == nil {
		t.Fatal("expected fill validation error")
	}
}
