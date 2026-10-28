package grayscale

import (
	"image"
	"math"
	"testing"
)

func TestResolveWorkingSet(t *testing.T) {
	for _, tc := range []struct {
		bounds image.Rectangle
		opts   Options
		want   int64
	}{
		{image.Rect(0, 0, 32, 16), Options{}, 6144},
		{image.Rect(7, 9, 39, 25), Options{Levels: 16, Dither: DitherFS}, 6688},
		{image.Rectangle{}, Options{}, 0},
		{image.Rect(0, 0, math.MaxInt, 2), Options{}, 0},
		{image.Rect(0, 0, 1, 1), Options{Levels: 3}, 0},
	} {
		t.Run(tc.bounds.String(), func(t *testing.T) {
			got, err := ResolveWorkingSet(tc.bounds, tc.opts)
			if tc.want == 0 {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %d %v want %d", got, err, tc.want)
			}
		})
	}
}
