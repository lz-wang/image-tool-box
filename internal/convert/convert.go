package convert

import (
	"fmt"
	"image/color"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
	"imagetoolbox/internal/imageio"
)

type Options struct {
	To         string
	Quality    int
	Lossless   bool
	Background string
}

func ConvertFile(inputPath, outputPath string, opts Options) error {
	format, err := imageio.NormalizeFormat(opts.To)
	if err != nil {
		return err
	}

	img, err := imaging.Open(inputPath)
	if err != nil {
		return fmt.Errorf("open input image: %w", err)
	}

	background := color.NRGBA{255, 255, 255, 255}
	if strings.TrimSpace(opts.Background) != "" {
		background, err = imageio.ParseHexColor(opts.Background)
		if err != nil {
			return fmt.Errorf("invalid background color: %w", err)
		}
	}

	return imageio.SaveWithFormat(outputPath, img, format, imageio.SaveOptions{
		Quality:    opts.Quality,
		Lossless:   opts.Lossless,
		Background: background,
		Flatten:    format == imageio.FormatJPEG || (format == imageio.FormatWEBP && !opts.Lossless),
	})
}

func DefaultOutputPath(inputPath string, to string) (string, error) {
	format, err := imageio.NormalizeFormat(to)
	if err != nil {
		return "", err
	}

	ext := "." + string(format)
	baseExt := filepath.Ext(inputPath)
	baseName := strings.TrimSuffix(filepath.Base(inputPath), baseExt)
	dir := filepath.Dir(inputPath)
	return filepath.Join(dir, baseName+"_converted"+ext), nil
}
