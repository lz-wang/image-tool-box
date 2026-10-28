package grayscale

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"os"
	"path/filepath"

	"imagetoolbox/internal/imageio"
)

// Apply composites alpha onto Background and computes BT.601-style encoded luma.
// The output has a zero origin and retains the input dimensions.
func Apply(src image.Image, opts Options) (*image.Gray, error) {
	opts.Normalize()
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if src == nil {
		return nil, fmt.Errorf("input image must not be nil")
	}
	bounds := src.Bounds()
	if bounds.Empty() {
		return nil, fmt.Errorf("image dimensions must be positive")
	}
	bg, err := imageio.ParseHexColor(opts.Background)
	if err != nil {
		return nil, err
	}
	dst := image.NewGray(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			c := pixel(src, bounds.Min.X+x, bounds.Min.Y+y)
			a := float64(c.A) / 255
			r := a*float64(c.R) + (1-a)*float64(bg.R)
			g := a*float64(c.G) + (1-a)*float64(bg.G)
			b := a*float64(c.B) + (1-a)*float64(bg.B)
			dst.Pix[y*dst.Stride+x] = uint8(math.Round(0.299*r + 0.587*g + 0.114*b))
		}
	}
	dither(dst, opts)
	return dst, nil
}

// pixel avoids boxing a new color interface per pixel for common decoder types.
func pixel(src image.Image, x, y int) color.NRGBA {
	switch src := src.(type) {
	case *image.NRGBA:
		return src.NRGBAAt(x, y)
	case *image.Gray:
		v := src.GrayAt(x, y).Y
		return color.NRGBA{R: v, G: v, B: v, A: 255}
	case *image.RGBA:
		c := src.RGBAAt(x, y)
		if c.A == 0 {
			return color.NRGBA{}
		}
		if c.A == 255 {
			return color.NRGBA{R: c.R, G: c.G, B: c.B, A: 255}
		}
		return color.NRGBA{R: uint8((uint32(c.R) * 65535 / uint32(c.A)) >> 8), G: uint8((uint32(c.G) * 65535 / uint32(c.A)) >> 8), B: uint8((uint32(c.B) * 65535 / uint32(c.A)) >> 8), A: c.A}
	case *image.YCbCr:
		c := src.YCbCrAt(x, y)
		r, g, b := color.YCbCrToRGB(c.Y, c.Cb, c.Cr)
		return color.NRGBA{R: r, G: g, B: b, A: 255}
	default:
		return color.NRGBAModel.Convert(src.At(x, y)).(color.NRGBA)
	}
}

// ConvertFile decodes JPEG/PNG/WebP with EXIF normalization and commits a fully
// encoded temporary file. Existing destinations survive all pre-commit failures.
// PNG is Gray8; JPEG/WebP promise grayscale pixels, not a single-channel codec.
// New files keep CreateTemp's private permissions (0600 before umask on Unix);
// replacement of an existing regular file preserves its permission bits.
func ConvertFile(inputPath, outputPath string, opts Options) error {
	opts.Normalize()
	if err := opts.Validate(); err != nil {
		return err
	}
	format, err := imageio.FormatFromPath(outputPath)
	if err != nil {
		return err
	}
	if opts.Lossless && format == imageio.FormatJPEG {
		return fmt.Errorf("%w: lossless is only supported for png and webp", ErrInvalidOptions)
	}
	if err := imageio.RejectSameFile(inputPath, outputPath); err != nil {
		return err
	}
	src, err := imageio.OpenStatic(inputPath)
	if err != nil {
		return err
	}
	dst, err := Apply(src, opts)
	if err != nil {
		return err
	}
	return commitFile(inputPath, outputPath, func(w io.Writer) error {
		return imageio.Encode(w, dst, format, imageio.SaveOptions{Quality: opts.Quality, Lossless: opts.Lossless})
	})
}

// commitFile stages the full encoding before touching an existing destination.
func commitFile(inputPath, outputPath string, encode func(io.Writer) error) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(outputPath), ".itb-grayscale-*")
	if err != nil {
		return err
	}
	defer func() {
		// Cleanup is best-effort; explicit Close below reports write errors.
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	if err := encode(tmp); err != nil {
		return err
	}
	// Only an existing regular destination supplies permissions. New files and
	// replacements of symlinks retain the private CreateTemp mode. Lstat avoids
	// inheriting permissions from a symlink's unrelated target.
	if stat, err := os.Lstat(outputPath); err == nil {
		if stat.Mode().IsRegular() {
			if err := tmp.Chmod(stat.Mode().Perm()); err != nil {
				return err
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// Recheck aliases immediately before committing the result.
	if err := imageio.RejectSameFile(inputPath, outputPath); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), outputPath)
}
