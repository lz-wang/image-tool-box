package grayscale

import (
	"image"
	"math"
)

// Dither names deterministic quantization algorithms.
type Dither string

const (
	DitherNone    Dither = "none"
	DitherFS      Dither = "fs"
	DitherOrdered Dither = "ordered"
)

func quantize(value float64, levels int) uint8 {
	value = math.Max(0, math.Min(255, value))
	return uint8(math.Round(math.Round(value*float64(levels-1)/255) * 255 / float64(levels-1)))
}

func dither(dst *image.Gray, opts Options) {
	if opts.Levels == 256 {
		return
	}
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	if opts.Dither == DitherFS {
		// Guard cells keep edge diffusion out of the next row's actual pixels.
		current, next := make([]float64, w+2), make([]float64, w+2)
		for y := 0; y < h; y++ {
			start, end, step := 0, w, 1
			if y%2 == 1 {
				start, end, step = w-1, -1, -1
			}
			for x := start; x != end; x += step {
				i := y*dst.Stride + x
				v := math.Max(0, math.Min(255, float64(dst.Pix[i])+current[x+1]))
				q := quantize(v, opts.Levels)
				dst.Pix[i] = q
				err := v - float64(q)
				current[x+1+step] += err * 7 / 16
				next[x+1-step] += err * 3 / 16
				next[x+1] += err * 5 / 16
				next[x+1+step] += err / 16
			}
			current, next = next, current
			clear(next)
		}
		return
	}
	bayer := [4][4]float64{{0, 8, 2, 10}, {12, 4, 14, 6}, {3, 11, 1, 9}, {15, 7, 13, 5}}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := y*dst.Stride + x
			v := float64(dst.Pix[i])
			if opts.Dither == DitherOrdered {
				v += ((bayer[y%4][x%4]+0.5)/16 - 0.5) * 255 / float64(opts.Levels-1)
			}
			dst.Pix[i] = quantize(v, opts.Levels)
		}
	}
}
