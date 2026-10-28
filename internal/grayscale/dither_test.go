package grayscale

import (
	"bytes"
	"fmt"
	"image"
	"testing"
)

func TestQuantizationAndDither(t *testing.T) {
	for _, levels := range []int{2, 4, 16, 256} {
		for _, mode := range []Dither{DitherNone, DitherFS, DitherOrdered} {
			if levels == 256 && mode != DitherNone {
				continue
			}
			for _, size := range []image.Point{{1, 1}, {1, 17}, {17, 1}, {256, 16}} {
				t.Run(fmt.Sprintf("%d/%s/%v", levels, mode, size), func(t *testing.T) {
					src := image.NewGray(image.Rect(0, 0, size.X, size.Y))
					for i := range src.Pix {
						src.Pix[i] = uint8(i)
					}
					opts := Options{Levels: levels, Dither: mode}
					got, err := Apply(src, opts)
					if err != nil {
						t.Fatal(err)
					}
					again, err := Apply(src, opts)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(got.Pix, again.Pix) {
						t.Fatal("not deterministic")
					}
					for _, v := range got.Pix {
						if int(v)%(255/(levels-1)) != 0 {
							t.Fatalf("non-palette value %d", v)
						}
					}
					if levels == 256 && !bytes.Equal(src.Pix, got.Pix) {
						t.Fatal("gray values changed")
					}
				})
			}
		}
	}
}

func TestDitherGolden(t *testing.T) {
	for _, tc := range []struct {
		mode Dither
		want []byte
	}{
		{DitherNone, []byte{0, 0, 0, 0, 0, 0, 0, 0}},
		{DitherFS, []byte{0, 255, 0, 255, 255, 0, 255, 0}},
		{DitherOrdered, []byte{0, 255, 0, 255, 255, 0, 255, 0}},
	} {
		t.Run(string(tc.mode), func(t *testing.T) {
			src := image.NewGray(image.Rect(0, 0, 4, 2))
			for i := range src.Pix {
				src.Pix[i] = 127
			}
			got, err := Apply(src, Options{Levels: 2, Dither: tc.mode})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got.Pix, tc.want) {
				t.Fatalf("got %v want %v", got.Pix, tc.want)
			}
		})
	}
}

func TestQuantizationGradient(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 256, 1))
	for i := range src.Pix {
		src.Pix[i] = uint8(i)
	}
	for _, levels := range []int{2, 4, 16, 256} {
		t.Run(fmt.Sprint(levels), func(t *testing.T) {
			got, err := Apply(src, Options{Levels: levels})
			if err != nil {
				t.Fatal(err)
			}
			for i, v := range got.Pix {
				want := uint8(((i*(levels-1) + 127) / 255) * (255 / (levels - 1)))
				if v != want {
					t.Fatalf("input %d got %d want %d", i, v, want)
				}
			}
		})
	}
}

func BenchmarkApply(b *testing.B) {
	src := image.NewGray(image.Rect(0, 0, 1024, 768))
	for _, mode := range []Dither{DitherNone, DitherFS, DitherOrdered} {
		b.Run(string(mode), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(src.Pix)))
			for b.Loop() {
				if _, err := Apply(src, Options{Levels: 16, Dither: mode}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
