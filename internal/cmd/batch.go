package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"imagetoolbox/internal/batch"
	"imagetoolbox/internal/convert"
	"imagetoolbox/internal/resize"
	"imagetoolbox/internal/watermark"
)

type batchCommonOptions struct {
	inputDir     string
	outputDir    string
	glob         string
	recursive    bool
	workers      int
	skipExisting bool
	failFast     bool
}

var (
	batchResizeOpts    batchCommonOptions
	batchConvertOpts   batchCommonOptions
	batchWatermarkOpts batchCommonOptions
)

var batchCmd = &cobra.Command{
	Use:   "batch",
	Short: "批量处理图片",
}

var batchResizeCmd = &cobra.Command{
	Use:   "resize",
	Short: "批量调整图片尺寸",
	RunE:  runBatchResize,
}

var batchConvertCmd = &cobra.Command{
	Use:   "convert",
	Short: "批量转换图片格式",
	RunE:  runBatchConvert,
}

var batchWatermarkCmd = &cobra.Command{
	Use:   "watermark",
	Short: "批量添加水印",
	RunE:  runBatchWatermark,
}

func init() {
	rootCmd.AddCommand(batchCmd)
	batchCmd.AddCommand(batchResizeCmd, batchConvertCmd, batchWatermarkCmd)

	addBatchCommonFlags(batchResizeCmd, &batchResizeOpts)
	addBatchCommonFlags(batchConvertCmd, &batchConvertOpts)
	addBatchCommonFlags(batchWatermarkCmd, &batchWatermarkOpts)

	batchResizeCmd.Flags().IntVar(&resizeWidth, "width", 0, "目标宽度")
	batchResizeCmd.Flags().IntVar(&resizeHeight, "height", 0, "目标高度")
	batchResizeCmd.Flags().StringVar(&resizePercent, "percent", "", "按比例缩放，例如 50%")
	batchResizeCmd.Flags().StringVar(&resizeMode, "mode", "fit", "缩放模式: fit/fill/stretch")
	batchResizeCmd.Flags().StringVar(&resizeAnchor, "anchor", "center", "填充模式锚点")
	batchResizeCmd.Flags().StringVar(&resizeFilter, "filter", "lanczos", "采样器: nearest/linear/catmullrom/lanczos")

	batchConvertCmd.Flags().StringVar(&convertTo, "to", "", "目标格式: jpg/jpeg/png/webp")
	batchConvertCmd.Flags().IntVarP(&convertQuality, "quality", "q", 80, "输出质量 (1-100)")
	batchConvertCmd.Flags().BoolVar(&convertLossless, "lossless", false, "使用无损编码（webp/png）")
	batchConvertCmd.Flags().StringVar(&convertBackground, "background", "#FFFFFF", "透明图转不透明格式时的背景色")
	batchConvertCmd.MarkFlagRequired("to")

	batchWatermarkCmd.Flags().StringVarP(&wmText, "text", "t", "", "水印文字")
	batchWatermarkCmd.Flags().StringVar(&wmImagePath, "image", "", "图片水印路径")
	batchWatermarkCmd.Flags().StringVarP(&wmMode, "mode", "m", "position", "水印模式: position/repeat")
	batchWatermarkCmd.Flags().StringVar(&wmColor, "color", "", "水印颜色")
	batchWatermarkCmd.Flags().IntVar(&wmSpace, "space", 0, "平铺间距")
	batchWatermarkCmd.Flags().IntVar(&wmAngle, "angle", 30, "旋转角度")
	batchWatermarkCmd.Flags().Float64Var(&wmOpacity, "opacity", 0.5, "透明度 (0~1)")
	batchWatermarkCmd.Flags().StringVar(&wmFontPath, "font", "", "字体文件路径")
	batchWatermarkCmd.Flags().IntVar(&wmFontSize, "font-size", 0, "字体大小")
	batchWatermarkCmd.Flags().StringVar(&wmPosition, "position", "bottom-right", "水印位置")
	batchWatermarkCmd.Flags().Float64Var(&wmMargin, "margin", 0.04, "边距比例")
	batchWatermarkCmd.Flags().Float64Var(&wmScale, "scale", 0.2, "图片水印缩放比例")
	batchWatermarkCmd.Flags().BoolVar(&wmTile, "tile", false, "图片水印平铺（当前版本暂不支持）")
}

func addBatchCommonFlags(cmd *cobra.Command, opts *batchCommonOptions) {
	cmd.Flags().StringVar(&opts.inputDir, "input-dir", "", "输入目录")
	cmd.Flags().StringVar(&opts.outputDir, "output-dir", "", "输出目录")
	cmd.Flags().StringVar(&opts.glob, "glob", "*", "文件匹配模式")
	cmd.Flags().BoolVar(&opts.recursive, "recursive", false, "递归处理子目录")
	cmd.Flags().IntVar(&opts.workers, "workers", 4, "并发 worker 数")
	cmd.Flags().BoolVar(&opts.skipExisting, "skip-existing", false, "跳过已存在输出")
	cmd.Flags().BoolVar(&opts.failFast, "fail-fast", false, "遇到错误时尽快停止")
	cmd.MarkFlagRequired("input-dir")
	cmd.MarkFlagRequired("output-dir")
}

func runBatchResize(cmd *cobra.Command, args []string) error {
	result, err := batch.Process(toBatchOptions(batchResizeOpts), resizeOutputRelPath, func(inputPath, outputPath string) error {
		return resize.ResizeFile(inputPath, outputPath, resize.Options{
			Width:   resizeWidth,
			Height:  resizeHeight,
			Percent: resizePercent,
			Mode:    resize.Mode(resizeMode),
			Anchor:  resizeAnchor,
			Filter:  resizeFilter,
		})
	})
	return finishBatch("resize", result, err)
}

func runBatchConvert(cmd *cobra.Command, args []string) error {
	result, err := batch.Process(toBatchOptions(batchConvertOpts), convertOutputRelPath, func(inputPath, outputPath string) error {
		return convert.ConvertFile(inputPath, outputPath, convert.Options{
			To:         convertTo,
			Quality:    convertQuality,
			Lossless:   convertLossless,
			Background: convertBackground,
		})
	})
	return finishBatch("convert", result, err)
}

func runBatchWatermark(cmd *cobra.Command, args []string) error {
	if err := validateWatermarkInput(); err != nil {
		return err
	}

	result, err := batch.Process(toBatchOptions(batchWatermarkOpts), watermarkOutputRelPath, func(inputPath, outputPath string) error {
		if wmImagePath != "" {
			if wmMode != "position" {
				return fmt.Errorf("图片水印仅支持 position 模式")
			}
			if wmTile {
				return fmt.Errorf("图片平铺水印暂不支持")
			}
			_, err := watermark.AddImageWatermark(inputPath, outputPath, &watermark.ImageOptions{
				ImagePath:   wmImagePath,
				Opacity:     &wmOpacity,
				Position:    watermark.Position(wmPosition),
				ScaleRatio:  &wmScale,
				MarginRatio: &wmMargin,
			})
			return err
		}

		switch wmMode {
		case "repeat":
			_, err := watermark.AddRepeatWatermark(inputPath, outputPath, wmText, &watermark.RepeatOptions{
				Color:          &wmColor,
				Space:          &wmSpace,
				Angle:          &wmAngle,
				Opacity:        &wmOpacity,
				FontPath:       wmFontPath,
				FontSize:       &wmFontSize,
				FontHeightCrop: nil,
			})
			return err
		case "position":
			_, err := watermark.AddPositionWatermark(inputPath, outputPath, wmText, &watermark.PositionOptions{
				Opacity:     &wmOpacity,
				Position:    watermark.Position(wmPosition),
				FontPath:    wmFontPath,
				FontSize:    &wmFontSize,
				Color:       &wmColor,
				MarginRatio: &wmMargin,
			})
			return err
		default:
			return fmt.Errorf("不支持的水印模式: %s", wmMode)
		}
	})
	return finishBatch("watermark", result, err)
}

func toBatchOptions(opts batchCommonOptions) batch.Options {
	return batch.Options{
		InputDir:     opts.inputDir,
		OutputDir:    opts.outputDir,
		Glob:         opts.glob,
		Recursive:    opts.recursive,
		Workers:      opts.workers,
		SkipExisting: opts.skipExisting,
		FailFast:     opts.failFast,
	}
}

func resizeOutputRelPath(rel string) string {
	ext := filepath.Ext(rel)
	base := strings.TrimSuffix(filepath.Base(rel), ext)
	dir := filepath.Dir(rel)
	return filepath.Join(dir, base+"_resized"+ext)
}

func convertOutputRelPath(rel string) string {
	ext := "." + normalizedFormatExt(convertTo)
	base := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
	dir := filepath.Dir(rel)
	return filepath.Join(dir, base+"_converted"+ext)
}

func watermarkOutputRelPath(rel string) string {
	ext := filepath.Ext(rel)
	base := strings.TrimSuffix(filepath.Base(rel), ext)
	dir := filepath.Dir(rel)
	return filepath.Join(dir, base+"_watermarked"+ext)
}

func normalizedFormatExt(to string) string {
	switch strings.ToLower(to) {
	case "jpg", "jpeg":
		return "jpeg"
	default:
		return strings.ToLower(strings.TrimPrefix(to, "."))
	}
}

func finishBatch(name string, result batch.Result, err error) error {
	fmt.Printf("%s 完成: success=%d skipped=%d failed=%d\n", name, result.Success, result.Skipped, result.Failed)
	for _, item := range result.Errors {
		fmt.Printf("  - %s: %v\n", item.Path, item.Err)
	}
	return err
}
