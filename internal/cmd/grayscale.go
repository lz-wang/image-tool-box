package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/urfave/cli/v3"
	"imagetoolbox/internal/grayscale"
	"imagetoolbox/internal/imageio"
)

func newGrayscaleCommand() *cli.Command {
	return &cli.Command{
		Name:      "grayscale",
		Usage:     "Convert an image to grayscale with optional quantization and dithering",
		Category:  categoryImageTransforms,
		ArgsUsage: "<src> [dst]",
		Description: `Convert a JPEG, PNG, or WebP image to opaque grayscale.

DEFAULTS:
  Writes <name>_grayscale.png, with 256 levels and no dithering.
  Alpha is composited onto white before BT.601-style luma conversion.

CONSTRAINTS:
  --levels accepts 2/4/16/256; --dither accepts none/fs/ordered.
  Dithering requires fewer than 256 levels. FS uses serpentine scanning;
  ordered uses a fixed 4x4 Bayer matrix. Resize before dithering.
  --background must be opaque (#RGB/#RRGGBB/#RRGGBBAA).
  Output format comes from [dst]: .png/.jpg/.jpeg/.webp.
  PNG uses Gray8 even with fewer levels; JPEG/WebP may store multiple channels
  and lossy encoding may change the quantized values.
  --lossless is allowed for PNG/WebP; PNG ignores quality.
  Dimensions are unchanged; JPEG EXIF Orientation is normalized.
  Output must differ from input, including hard links and symlinks.
  Success is text; --format json changes failure output only.

EXAMPLES:
  itb grayscale photo.jpg
  itb grayscale --levels 16 --dither fs photo.jpg eink.png
  itb grayscale --levels 4 --dither ordered photo.png output.png
  itb grayscale --background "#FFFFFF" photo.webp output.png`,
		Flags: []cli.Flag{
			&cli.IntFlag{Name: "levels", Value: grayscale.DefaultLevels, Usage: "Number of gray levels: 2/4/16/256", Validator: grayscale.ValidateLevels},
			&cli.StringFlag{Name: "dither", Value: string(grayscale.DitherNone), Usage: "Dither algorithm: none/fs/ordered"},
			&cli.StringFlag{Name: "background", Value: grayscale.DefaultBackground, Usage: "Opaque alpha-compositing background COLOR"},
			&cli.IntFlag{Name: "quality", Aliases: []string{"q"}, Value: grayscale.DefaultQuality, Usage: "JPEG/WebP quality (1-100); effort for lossless WebP; ignored for PNG", Validator: grayscale.ValidateQuality},
			&cli.BoolFlag{Name: "lossless", Usage: "Use lossless WebP encoding; PNG is always lossless"},
			&cli.StringFlag{Name: "format", Value: "text", Usage: "Error output format: text/json (success remains text)", Validator: enumValidator("format", "text", "json")},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			src, dst, err := sourceDestinationArgs(cmd, false)
			if err == nil {
				if dst == "" {
					dst = filepath.Join(filepath.Dir(src), imageio.SuffixedName(src, "_grayscale", ".png"))
				}
				err = grayscale.ConvertFile(src, dst, grayscale.Options{Levels: cmd.Int("levels"), Dither: grayscale.Dither(cmd.String("dither")), Background: cmd.String("background"), Quality: cmd.Int("quality"), Lossless: cmd.Bool("lossless")})
			}
			if err != nil {
				return operationError("grayscale", err)
			}
			_, err = fmt.Fprintf(cmd.Writer, "灰度转换完成: %s\n", dst)
			return operationError("grayscale", err)
		},
	}
}
