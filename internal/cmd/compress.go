package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"
	"imagetoolbox/internal/compress"
	"imagetoolbox/internal/imageio"
)

func newCompressCommand() *cli.Command {
	return &cli.Command{
		Name:      "compress",
		Usage:     "Compress a PNG or JPEG image",
		Category:  categoryImageTransforms,
		ArgsUsage: "<src> [dst]",
		Description: `Compress a PNG or JPEG image. The input format is
detected from the file header; no format flag is needed.

Pipelines:
  PNG:  pngquant -> oxipng
  JPEG: djpeg -> cjpeg (libjpeg-turbo)

DEFAULTS:
  If [dst] is omitted, writes <name>_compressed.<ext>.
  The input file is kept; only --in-place overwrites it.
  Output is staged to a temporary file and committed with an
  atomic rename: a failed run never leaves a partial file at
  the destination.

CONSTRAINTS:
  Supported input formats are PNG and JPEG only.
  --in-place cannot be combined with a [dst] operand.

EXAMPLES:
  itb compress photo.png
  itb compress -q 90 photo.jpg compressed.jpg
  itb compress --in-place photo.jpg
  itb compress --format json photo.png`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "in-place",
				Usage: "Overwrite the source file",
			},
			&cli.IntFlag{
				Name:      "quality",
				Aliases:   []string{"q"},
				Value:     compress.DefaultQuality,
				Usage:     "Compression quality (1-100)",
				Validator: intRangeValidator("quality", 1, 100),
			},
			&cli.StringFlag{
				Name:      "format",
				Value:     "table",
				Usage:     "Output `FORMAT`: table/json (JSON contract itb.compress.v1)",
				Validator: enumValidator("format", "table", "json"),
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return operationError("compress", runCompress(ctx, cmd))
		},
	}
}

func runCompress(ctx context.Context, cmd *cli.Command) error {
	inputFile, outputPath, err := sourceDestinationArgs(cmd, false)
	if err != nil {
		return err
	}
	if cmd.Bool("in-place") && outputPath != "" {
		return invalidArgument("--in-place 不能与 <dst> 同时使用")
	}
	tmpPath := ""
	if cmd.Bool("in-place") {
		// 临时文件放在输入文件所在目录，保证 rename 不跨文件系统；
		// 后缀区分于领域层的安全提交临时文件（.itb-compress-*）
		tmp, err := os.CreateTemp(filepath.Dir(inputFile), ".itb-inplace-*"+filepath.Ext(inputFile))
		if err != nil {
			return fmt.Errorf("创建临时文件失败: %w", err)
		}
		tmpPath = tmp.Name()
		tmp.Close()
		outputPath = tmpPath
	} else if outputPath == "" {
		outputPath = imageio.SuffixedPath(inputFile, "_compressed")
	}

	result, err := compress.CompressFile(ctx, inputFile, outputPath, compress.FileOptions{Quality: cmd.Int("quality")})
	if err != nil {
		if tmpPath != "" {
			os.Remove(tmpPath)
		}
		return err
	}

	if tmpPath != "" {
		if err := os.Rename(tmpPath, inputFile); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("覆盖原文件失败: %w", err)
		}
		outputPath = inputFile
	}

	if cmd.String("format") == "json" {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(compress.NewReport(inputFile, outputPath, result))
	}

	fmt.Printf("检测到格式: %s\n", result.Format)
	fmt.Printf("压缩完成: %s (%s → %s)\n", outputPath, formatSize(result.InputSize), formatSize(result.OutputSize))
	return nil
}

func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGTPE"[exp])
}
