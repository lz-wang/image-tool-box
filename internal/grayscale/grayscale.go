package grayscale

import (
	"fmt"
	"image"
	"image/color"
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
			c := color.NRGBAModel.Convert(src.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
			a := float64(c.A) / 255
			r := a*float64(c.R) + (1-a)*float64(bg.R)
			g := a*float64(c.G) + (1-a)*float64(bg.G)
			b := a*float64(c.B) + (1-a)*float64(bg.B)
			dst.Pix[y*dst.Stride+x] = uint8(math.Round(0.299*r + 0.587*g + 0.114*b))
		}
	}
	return dst, nil
}

// ConvertFile decodes JPEG/PNG/WebP with EXIF normalization and commits a fully
// encoded temporary file. Existing destinations survive all pre-commit failures.
// PNG is Gray8; JPEG/WebP promise grayscale pixels, not a single-channel codec.
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
		return fmt.Errorf("lossless is only supported for png and webp")
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
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(outputPath), ".itb-grayscale-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if err := imageio.Encode(tmp, dst, format, imageio.SaveOptions{Quality: opts.Quality, Lossless: opts.Lossless}); err != nil {
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
