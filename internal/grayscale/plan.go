package grayscale

import (
	"fmt"
	"image"
	"math"
)

// ResolveWorkingSet estimates image processing memory before decoding pixels.
// 12 bytes/pixel reserves space for decoding, EXIF normalization, Gray8 output
// and encoding; FS additionally needs two float64 rows including guard cells.
// This is an admission estimate, not a limit on codec/runtime allocations.
func ResolveWorkingSet(bounds image.Rectangle, opts Options) (int64, error) {
	opts.Normalize()
	if err := opts.Validate(); err != nil {
		return 0, err
	}
	w, h := int64(bounds.Dx()), int64(bounds.Dy())
	if w <= 0 || h <= 0 {
		return 0, fmt.Errorf("image dimensions must be positive")
	}
	var rows int64
	if opts.Dither == DitherFS {
		if w > math.MaxInt64/16-2 {
			return 0, fmt.Errorf("image working set overflows")
		}
		rows = 16 * (w + 2)
	}
	if w > (math.MaxInt64-rows)/12/h {
		return 0, fmt.Errorf("image working set overflows")
	}
	return 12*w*h + rows, nil
}
