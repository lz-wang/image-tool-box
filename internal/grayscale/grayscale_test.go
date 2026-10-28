package grayscale

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
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
			defer f.Close()
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
