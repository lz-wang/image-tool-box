package cmd

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"imagetoolbox/internal/compare"
)

func newCompareCommand() *cli.Command {
	return &cli.Command{
		Name:      "compare",
		Usage:     "Compare objective quality metrics of two images",
		Category:  categoryAnalysis,
		ArgsUsage: "<src> <dst>",
		Description: `Compare two images with objective quality metrics
(PSNR / SSIM / MS-SSIM).

<src> is the reference image.
<dst> is the comparison target, not an output path.
This command is read-only: it does not modify either input.
Comparing a file with itself is a valid sanity check (PSNR
is +Inf, SSIM / MS-SSIM are 1).

DEFAULTS:
  If no metric flag is specified, PSNR and MS-SSIM are
  computed.
  If any metric flag is specified, only explicitly enabled
  metrics are computed.

CONSTRAINTS:
  Supported formats are JPEG / PNG / WebP. JPEG EXIF
  Orientation is normalized, so metrics compare the actual
  visual pixels.
  Both images must have exactly the same logical dimensions;
  there is no implicit resize, crop, or padding.
  SSIM requires both images to be at least 11x11.
  MS-SSIM uses a fixed five-scale decomposition and requires
  the shortest side to be >= 161.

EXAMPLES:
  itb compare original.jpg compressed.jpg
  itb compare original.jpg compressed.jpg --ssim
  itb compare original.png original.webp --psnr --ssim --ms-ssim`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "psnr",
				Usage: "Compute PSNR (peak signal-to-noise ratio in dB; +Inf for identical images)",
			},
			&cli.BoolFlag{
				Name:  "ssim",
				Usage: "Compute SSIM (structural similarity; 11x11 Gaussian window, sigma 1.5)",
			},
			&cli.BoolFlag{
				Name:  "ms-ssim",
				Usage: "Compute MS-SSIM (fixed five-scale; shortest side must be >= 161 pixels)",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return operationError("compare", runCompare(ctx, cmd))
		},
	}
}

// resolveCompareMetrics 解析指标 flag 的 transport 语义：
//
//   - 没有任何指标 flag 被 IsSet 时返回 0，交给 domain Normalize
//     得到默认的 PSNR + MS-SSIM；
//   - 只要任一指标 flag 显式提供（含 =false），则只组合显式为 true
//     的指标；全部显式关闭时直接报错，绝不让 domain 的零值默认机制
//     偷偷恢复默认指标。
func resolveCompareMetrics(cmd *cli.Command) (compare.Metrics, error) {
	psnrSet := cmd.IsSet("psnr")
	ssimSet := cmd.IsSet("ssim")
	msSSIMSet := cmd.IsSet("ms-ssim")
	if !psnrSet && !ssimSet && !msSSIMSet {
		return 0, nil
	}

	var metrics compare.Metrics
	if cmd.Bool("psnr") {
		metrics |= compare.MetricPSNR
	}
	if cmd.Bool("ssim") {
		metrics |= compare.MetricSSIM
	}
	if cmd.Bool("ms-ssim") {
		metrics |= compare.MetricMSSSIM
	}
	if metrics == 0 {
		return 0, invalidArgument("至少需要选择一个比较指标")
	}
	return metrics, nil
}

func runCompare(ctx context.Context, cmd *cli.Command) error {
	src, dst, err := sourceDestinationArgs(cmd, true)
	if err != nil {
		return err
	}

	metrics, err := resolveCompareMetrics(cmd)
	if err != nil {
		return err
	}

	result, err := compare.CompareFiles(ctx, src, dst, compare.Options{Metrics: metrics})
	if err != nil {
		return fmt.Errorf("比较失败: %w", err)
	}

	// 输出顺序固定为 PSNR、SSIM、MS-SSIM，与 flag 出现顺序无关。
	if result.Metrics&compare.MetricPSNR != 0 {
		fmt.Printf("PSNR: %.6f dB\n", result.PSNR)
	}
	if result.Metrics&compare.MetricSSIM != 0 {
		fmt.Printf("SSIM: %.6f\n", result.SSIM)
	}
	if result.Metrics&compare.MetricMSSSIM != 0 {
		fmt.Printf("MS-SSIM: %.6f\n", result.MSSSIM)
	}
	return nil
}
