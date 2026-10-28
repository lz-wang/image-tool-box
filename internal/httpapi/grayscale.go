package httpapi

import (
	"context"
	"fmt"
	"image"
	"path/filepath"

	"imagetoolbox/internal/grayscale"
	"imagetoolbox/internal/imageio"
)

func grayscaleImage(_ context.Context, f form, dir string, cfg Config) (string, string, int64, error) {
	input, err := f.input("input", "levels", "dither", "background", "to", "quality", "lossless")
	if err != nil {
		return "", "", 0, err
	}
	levels, err := f.int("levels")
	if err != nil {
		return "", "", 0, err
	}
	quality, err := f.int("quality")
	if err != nil {
		return "", "", 0, err
	}
	if _, supplied := f.values["levels"]; supplied {
		if err := grayscale.ValidateLevels(levels); err != nil {
			return "", "", 0, err
		}
	}
	if _, supplied := f.values["quality"]; supplied {
		if err := grayscale.ValidateQuality(quality); err != nil {
			return "", "", 0, err
		}
	}
	lossless, err := f.bool("lossless")
	if err != nil {
		return "", "", 0, err
	}
	opts := grayscale.Options{Levels: levels, Dither: grayscale.Dither(f.values["dither"]), Background: f.values["background"], Quality: quality, Lossless: lossless}
	to := f.values["to"]
	if to == "" {
		to = "png"
	}
	format, err := imageio.NormalizeFormat(to)
	if err != nil {
		return "", "", 0, err
	}
	info, err := imageio.Probe(input.Path)
	if err != nil {
		return "", "", 0, err
	}
	working, err := grayscale.ResolveWorkingSet(image.Rect(0, 0, info.Width, info.Height), opts)
	if err != nil {
		return "", "", 0, err
	}
	if working > cfg.MaxWorkingBytes {
		return "", "", 0, fmt.Errorf("%w: grayscale working set exceeds %d bytes", ErrImageTooLarge, cfg.MaxWorkingBytes)
	}
	ext := "." + string(format)
	output := filepath.Join(dir, "result"+ext)
	err = grayscale.ConvertFile(input.Path, output, opts)
	return output, imageio.SuffixedName(input.OriginalName, "_grayscale", ext), fileSize(input.Path), err
}
