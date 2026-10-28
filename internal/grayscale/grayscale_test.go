package grayscale

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyLuma(t *testing.T) {
	for _, tc := range []struct {
		name string
		c    color.NRGBA
		bg   string
		want uint8
	}{
		{"black", color.NRGBA{0, 0, 0, 255}, "", 0},
		{"white", color.NRGBA{255, 255, 255, 255}, "", 255},
		{"red", color.NRGBA{255, 0, 0, 255}, "", 76},
		{"green", color.NRGBA{0, 255, 0, 255}, "", 150},
		{"blue", color.NRGBA{0, 0, 255, 255}, "", 29},
		{"transparent", color.NRGBA{255, 0, 0, 0}, "", 255},
		{"half", color.NRGBA{0, 0, 0, 128}, "", 127},
		{"background", color.NRGBA{0, 0, 0, 0}, "#F00", 76},
		{"gray", color.NRGBA{123, 123, 123, 255}, "", 123},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := image.NewNRGBA(image.Rect(7, 9, 8, 10))
			src.SetNRGBA(7, 9, tc.c)
			got, err := Apply(src, Options{Background: tc.bg})
			if err != nil {
				t.Fatal(err)
			}
			if got.Bounds() != image.Rect(0, 0, 1, 1) || got.GrayAt(0, 0).Y != tc.want {
				t.Fatalf("got %v %v, want %d", got.Bounds(), got.Pix, tc.want)
			}
		})
	}
}

func writeSource(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "input.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, image.NewNRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConvertFile(t *testing.T) {
	dir := t.TempDir()
	src := writeSource(t, dir)
	for _, ext := range []string{"png", "jpg", "webp"} {
		t.Run(ext, func(t *testing.T) {
			dst := filepath.Join(dir, "nested", "out."+ext)
			if err := ConvertFile(src, dst, Options{}); err != nil {
				t.Fatal(err)
			}
			f, err := os.Open(dst)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = f.Close() }() // Read-only descriptor; decode errors are checked below.
			img, _, err := image.Decode(f)
			if err != nil {
				t.Fatal(err)
			}
			if img.Bounds().Dx() != 3 || img.Bounds().Dy() != 2 {
				t.Fatal(img.Bounds())
			}
			if ext == "png" {
				if _, ok := img.(*image.Gray); !ok {
					t.Fatalf("not Gray8: %T", img)
				}
			}
		})
	}
	t.Run("aliases", func(t *testing.T) {
		for _, kind := range []string{"same", "hard", "symlink"} {
			t.Run(kind, func(t *testing.T) {
				dst := src
				if kind != "same" {
					dst = filepath.Join(dir, kind+".png")
					var err error
					if kind == "hard" {
						err = os.Link(src, dst)
					} else {
						err = os.Symlink(src, dst)
					}
					if err != nil {
						t.Skip(err)
					}
				}
				before, err := os.ReadFile(src)
				if err != nil {
					t.Fatal(err)
				}
				if err := ConvertFile(src, dst, Options{}); err == nil {
					t.Fatal("alias accepted")
				}
				after, err := os.ReadFile(src)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatal("source changed", err)
				}
			})
		}
	})
	t.Run("failure preserves target", func(t *testing.T) {
		dst := filepath.Join(dir, "keep.png")
		if err := os.WriteFile(dst, []byte("keep"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := ConvertFile(src, dst, Options{Quality: -1}); err == nil {
			t.Fatal("invalid quality accepted")
		}
		got, err := os.ReadFile(dst)
		if err != nil || string(got) != "keep" {
			t.Fatal(string(got), err)
		}
	})
}

func TestInvalidOptions(t *testing.T) {
	for _, opts := range []Options{{Levels: 3}, {Dither: "bad"}, {Dither: DitherFS}, {Quality: -1}, {Quality: 101}, {Background: "oops"}, {Background: "#00000000"}} {
		if _, err := Apply(image.NewGray(image.Rect(0, 0, 1, 1)), opts); err == nil {
			t.Fatalf("accepted %+v", opts)
		}
	}
	if _, err := Apply(nil, Options{}); err == nil {
		t.Fatal("nil accepted")
	}
	if _, err := Apply(image.NewGray(image.Rectangle{}), Options{}); err == nil {
		t.Fatal("empty accepted")
	}
}

func TestCommitFailureSafety(t *testing.T) {
	dir := t.TempDir()
	src := writeSource(t, dir)
	dst := filepath.Join(dir, "target.png")
	if err := os.WriteFile(dst, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("encoder stopped")
	err := commitFile(src, dst, func(w io.Writer) error {
		if _, err := w.Write([]byte("partial")); err != nil {
			return err
		}
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil || string(got) != "keep" {
		t.Fatal(string(got), err)
	}
	// A directory destination forces a failure after full encoding and close.
	blocked := filepath.Join(dir, "blocked.png")
	if err := os.Mkdir(blocked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ConvertFile(src, blocked, Options{}); err == nil {
		t.Fatal("directory replaced")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".itb-grayscale-") {
			t.Fatal("temporary file left behind", entry.Name())
		}
	}
}

type genericImage struct{ image.Image }

func TestPixelTypesMatchGenericConversion(t *testing.T) {
	bounds := image.Rect(7, 9, 14, 12)
	nrgba := image.NewNRGBA(bounds)
	rgba := image.NewRGBA(bounds)
	gray := image.NewGray(bounds)
	wide := image.NewNRGBA64(bounds)
	yuv := image.NewYCbCr(bounds, image.YCbCrSubsampleRatio420)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := color.NRGBA{R: uint8(x * 17), G: uint8(y * 13), B: 91, A: uint8((x - 7) * 42)}
			nrgba.SetNRGBA(x, y, c)
			rgba.Set(x, y, c)
			gray.Set(x, y, c)
			wide.Set(x, y, c)
		}
	}
	for i := range yuv.Y {
		yuv.Y[i] = uint8(i * 11)
	}
	for i := range yuv.Cb {
		yuv.Cb[i] = 100
		yuv.Cr[i] = 180
	}
	for name, src := range map[string]image.Image{"nrgba": nrgba, "rgba": rgba, "gray": gray, "64-bit": wide, "ycbcr": yuv} {
		t.Run(name, func(t *testing.T) {
			got, err := Apply(src, Options{})
			if err != nil {
				t.Fatal(err)
			}
			want, err := Apply(genericImage{src}, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got.Pix, want.Pix) {
				t.Fatalf("pixel conversion differs: %v vs %v", got.Pix, want.Pix)
			}
		})
	}
}
