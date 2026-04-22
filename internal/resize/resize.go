package resize

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"

	"github.com/disintegration/imaging"
	"imagetoolbox/internal/imageio"
)

type Mode string

const (
	ModeFit     Mode = "fit"
	ModeFill    Mode = "fill"
	ModeStretch Mode = "stretch"
)

type Options struct {
	Width   int
	Height  int
	Percent string
	Mode    Mode
	Anchor  string
	Filter  string
}

func ResizeFile(inputPath, outputPath string, opts Options) error {
	img, err := imaging.Open(inputPath)
	if err != nil {
		return fmt.Errorf("open input image: %w", err)
	}

	resized, err := Apply(img, opts)
	if err != nil {
		return err
	}

	return imageio.Save(outputPath, resized, imageio.SaveOptions{
		Quality:    100,
		Background: imageioMustColor("#FFFFFF"),
	})
}

func Apply(img image.Image, opts Options) (image.Image, error) {
	width, height, mode, anchor, filter, err := normalize(img.Bounds(), opts)
	if err != nil {
		return nil, err
	}

	switch mode {
	case ModeFit:
		if width == 0 || height == 0 {
			return imaging.Resize(img, width, height, filter), nil
		}
		return imaging.Fit(img, width, height, filter), nil
	case ModeFill:
		return imaging.Fill(img, width, height, anchor, filter), nil
	case ModeStretch:
		return imaging.Resize(img, width, height, filter), nil
	default:
		return nil, fmt.Errorf("unsupported resize mode: %s", mode)
	}
}

func normalize(bounds image.Rectangle, opts Options) (int, int, Mode, imaging.Anchor, imaging.ResampleFilter, error) {
	mode := opts.Mode
	if mode == "" {
		mode = ModeFit
	}

	filter, err := parseFilter(opts.Filter)
	if err != nil {
		return 0, 0, "", imaging.Center, imaging.Lanczos, err
	}
	anchor, err := parseAnchor(opts.Anchor)
	if err != nil {
		return 0, 0, "", imaging.Center, imaging.Lanczos, err
	}

	if opts.Percent != "" && (opts.Width > 0 || opts.Height > 0) {
		return 0, 0, "", imaging.Center, imaging.Lanczos, fmt.Errorf("--percent cannot be used together with --width or --height")
	}

	if opts.Percent == "" && opts.Width <= 0 && opts.Height <= 0 {
		return 0, 0, "", imaging.Center, imaging.Lanczos, fmt.Errorf("must provide --percent or at least one of --width/--height")
	}

	width := opts.Width
	height := opts.Height
	if opts.Percent != "" {
		percent, err := parsePercent(opts.Percent)
		if err != nil {
			return 0, 0, "", imaging.Center, imaging.Lanczos, err
		}
		width = max(1, int(math.Round(float64(bounds.Dx())*percent/100)))
		height = max(1, int(math.Round(float64(bounds.Dy())*percent/100)))
	}

	switch mode {
	case ModeFit, ModeStretch:
		if width <= 0 && height <= 0 {
			return 0, 0, "", imaging.Center, imaging.Lanczos, fmt.Errorf("resize target size is invalid")
		}
	case ModeFill:
		if width <= 0 || height <= 0 {
			return 0, 0, "", imaging.Center, imaging.Lanczos, fmt.Errorf("--mode fill requires both --width and --height")
		}
	default:
		return 0, 0, "", imaging.Center, imaging.Lanczos, fmt.Errorf("unsupported resize mode: %s", mode)
	}

	return width, height, mode, anchor, filter, nil
}

func parsePercent(value string) (float64, error) {
	if !strings.HasSuffix(value, "%") {
		return 0, fmt.Errorf("percent must use %% suffix, for example 50%%")
	}
	numberPart := strings.TrimSuffix(value, "%")
	parsed, err := strconv.ParseFloat(numberPart, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid percent: %s", value)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("percent must be greater than 0: %s", value)
	}
	return parsed, nil
}

func parseFilter(value string) (imaging.ResampleFilter, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "lanczos":
		return imaging.Lanczos, nil
	case "nearest":
		return imaging.NearestNeighbor, nil
	case "linear":
		return imaging.Linear, nil
	case "catmullrom":
		return imaging.CatmullRom, nil
	default:
		return imaging.Lanczos, fmt.Errorf("unsupported filter: %s", value)
	}
}

func parseAnchor(value string) (imaging.Anchor, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "center":
		return imaging.Center, nil
	case "left":
		return imaging.Left, nil
	case "right":
		return imaging.Right, nil
	case "top":
		return imaging.Top, nil
	case "bottom":
		return imaging.Bottom, nil
	case "top-left":
		return imaging.TopLeft, nil
	case "top-right":
		return imaging.TopRight, nil
	case "bottom-left":
		return imaging.BottomLeft, nil
	case "bottom-right":
		return imaging.BottomRight, nil
	default:
		return imaging.Center, fmt.Errorf("unsupported anchor: %s", value)
	}
}

func imageioMustColor(hex string) color.NRGBA {
	col, err := imageio.ParseHexColor(hex)
	if err != nil {
		panic(err)
	}
	return col
}
