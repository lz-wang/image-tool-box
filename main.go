package main

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "image/jpeg"
	_ "image/png"

	"github.com/spf13/cobra"
	"imagetoolbox/internal/compress"
	"imagetoolbox/internal/watermark"
)

//go:embed bins/**
var binaries embed.FS

var (
	version = "dev"
)

func main() {
	compress.InitBinaries(binaries)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "imagetoolbox",
	Short: "高效的图片压缩工具",
	Long: `一个基于 pngquant、oxipng 和 libjpeg-turbo 的图片压缩 CLI 工具。

支持 PNG 和 JPEG 格式的高效压缩，所有依赖二进制已内嵌，无需外部依赖。`,
}

var compressCmd = &cobra.Command{
	Use:   "compress",
	Short: "自动检测并压缩图片",
	Long: `自动检测输入图片的格式（PNG/JPEG），然后执行对应的压缩操作。

无需指定图片类型，程序会通过读取文件头自动判断。`,
	Example: `  imagetoolbox compress -i photo.png
  imagetoolbox compress -i photo.jpg -o compressed.jpg -q 90`,
	RunE: runCompress,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "显示版本信息",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("imagetoolbox version %s\n", version)
	},
}

var watermarkCmd = &cobra.Command{
	Use:   "watermark",
	Short: "为图片添加水印",
	Long: `为图片添加文字水印，支持两种模式：

1. position（默认）: 单点位置水印，在指定位置添加水印
   - 自动根据背景亮度选择黑/白文字
   - 支持自动描边提高可读性

2. repeat: 重复平铺水印，文字以平铺方式覆盖整张图片
   - 支持旋转角度和间距调整
   - 需要指定字体文件路径`,
	Example: `  # 位置水印（默认右下角，智能颜色）
  imagetoolbox watermark -i photo.jpg -t "Author"

  # 指定位置和透明度
  imagetoolbox watermark -i photo.png -t "Copyright" -p center --opacity 0.8

  # 重复平铺水印
  imagetoolbox watermark -i photo.png -t "WATERMARK" --mode repeat --font /path/to/font.ttf

  # 指定输出路径
  imagetoolbox watermark -i photo.jpg -t "Author" -o output.jpg`,
	RunE: runWatermark,
}

var (
	inputFile  string
	outputFile string
	quality    int
)

// watermark 命令参数
var (
	wmInputFile  string
	wmOutputFile string
	wmText       string
	wmMode       string
	wmColor      string
	wmSpace      int
	wmAngle      int
	wmOpacity    float64
	wmFontPath   string
	wmFontSize   int
	wmPosition   string
	wmMargin     float64
)

func init() {
	rootCmd.AddCommand(compressCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(watermarkCmd)

	compressCmd.Flags().StringVarP(&inputFile, "input", "i", "", "输入图片文件路径")
	compressCmd.Flags().StringVarP(&outputFile, "output", "o", "", "输出图片文件路径")
	compressCmd.Flags().IntVarP(&quality, "quality", "q", 80, "压缩质量 (1-100)")

	// watermark 命令参数
	watermarkCmd.Flags().StringVarP(&wmInputFile, "input", "i", "", "输入图片文件路径")
	watermarkCmd.Flags().StringVarP(&wmOutputFile, "output", "o", "", "输出图片文件路径（默认在原文件名后加 _watermarked）")
	watermarkCmd.Flags().StringVarP(&wmText, "text", "t", "", "水印文字")
	watermarkCmd.Flags().StringVarP(&wmMode, "mode", "m", "position", "水印模式: position（位置）/ repeat（重复平铺）")
	watermarkCmd.Flags().StringVar(&wmColor, "color", "#4db6ac", "水印颜色（repeat模式）")
	watermarkCmd.Flags().IntVar(&wmSpace, "space", 75, "平铺间距（repeat模式）")
	watermarkCmd.Flags().IntVar(&wmAngle, "angle", 30, "旋转角度（repeat模式）")
	watermarkCmd.Flags().Float64Var(&wmOpacity, "opacity", 0.5, "透明度 (0~1)")
	watermarkCmd.Flags().StringVar(&wmFontPath, "font", "", "字体文件路径")
	watermarkCmd.Flags().IntVar(&wmFontSize, "font-size", 48, "字体大小（repeat模式）")
	watermarkCmd.Flags().StringVarP(&wmPosition, "position", "p", "bottom-right", "水印位置: bottom-right/bottom-left/top-right/top-left/center")
	watermarkCmd.Flags().Float64Var(&wmMargin, "margin", 0.04, "边距比例（position模式）")

	watermarkCmd.MarkFlagRequired("input")
	watermarkCmd.MarkFlagRequired("text")
}

func runCompress(cmd *cobra.Command, args []string) error {
	if inputFile == "" {
		return fmt.Errorf("必须指定输入文件路径 (-i)")
	}

	f, err := os.Open(inputFile)
	if err != nil {
		return fmt.Errorf("无法打开输入文件: %w", err)
	}

	format, err := compress.DetectFormat(f)
	f.Close()
	if err != nil {
		return fmt.Errorf("无法检测图片格式: %w", err)
	}

	fmt.Printf("检测到格式: %s\n", format)

	switch format {
	case "png":
		return compressPNGFile(inputFile, outputFile, quality)
	case "jpeg":
		return compressJPEGFile(inputFile, outputFile, quality)
	default:
		return fmt.Errorf("不支持的图片格式: %s", format)
	}
}

func compressPNGFile(inPath, outPath string, q int) error {
	input, err := os.Open(inPath)
	if err != nil {
		return err
	}
	defer input.Close()

	var output *os.File
	var outputPath string
	var tmpFile *os.File

	if outPath != "" {
		output, err = os.Create(outPath)
		if err != nil {
			return err
		}
		defer output.Close()
		outputPath = outPath
	} else {
		tmpFile, err = os.CreateTemp("", "imagetoolbox-*.png")
		if err != nil {
			return err
		}
		output = tmpFile
		outputPath = inPath
	}

	opts := compress.PNGOptions{
		Quality:     q,
		OxiPngLevel: 4,
		Input:       input,
		Output:      output,
	}

	if err := compress.CompressPNG(opts); err != nil {
		return err
	}

	if tmpFile != nil {
		tmpFile.Close()
		os.Rename(tmpFile.Name(), inPath)
	}

	fmt.Printf("压缩完成: %s\n", outputPath)
	return nil
}

func compressJPEGFile(inPath, outPath string, q int) error {
	var output *os.File
	var outputPath string
	var tmpFile *os.File
	var err error

	if outPath != "" {
		output, err = os.Create(outPath)
		if err != nil {
			return err
		}
		defer output.Close()
		outputPath = outPath
	} else {
		tmpFile, err = os.CreateTemp("", "imagetoolbox-*.jpg")
		if err != nil {
			return err
		}
		output = tmpFile
		outputPath = inPath
	}

	opts := compress.JPEGOptions{
		Quality:     q,
		Progressive: true,
		Optimize:    true,
		InputPath:   inPath,
		Output:      output,
	}

	if err := compress.CompressJPEG(opts); err != nil {
		return err
	}

	if tmpFile != nil {
		tmpFile.Close()
		os.Rename(tmpFile.Name(), inPath)
	}

	fmt.Printf("压缩完成: %s\n", outputPath)
	return nil
}

func runWatermark(cmd *cobra.Command, args []string) error {
	if wmInputFile == "" {
		return fmt.Errorf("必须指定输入文件路径 (-i)")
	}
	if wmText == "" {
		return fmt.Errorf("必须指定水印文字 (-t)")
	}

	// 生成默认输出路径
	outputPath := wmOutputFile
	if outputPath == "" {
		ext := filepath.Ext(wmInputFile)
		base := strings.TrimSuffix(filepath.Base(wmInputFile), ext)
		dir := filepath.Dir(wmInputFile)
		outputPath = filepath.Join(dir, base+"_watermarked"+ext)
	}

	var err error
	switch wmMode {
	case "repeat":
		opts := &watermark.RepeatOptions{
			Color:          &wmColor,
			Space:          &wmSpace,
			Angle:          &wmAngle,
			Opacity:        &wmOpacity,
			FontPath:       wmFontPath,
			FontSize:       &wmFontSize,
			FontHeightCrop: nil,
		}
		_, err = watermark.AddRepeatWatermark(wmInputFile, outputPath, wmText, opts)

	case "position":
		pos := watermark.Position(wmPosition)
		opts := &watermark.PositionOptions{
			Opacity:     &wmOpacity,
			Position:    pos,
			FontPath:    wmFontPath,
			MarginRatio: &wmMargin,
		}
		_, err = watermark.AddPositionWatermark(wmInputFile, outputPath, wmText, opts)

	default:
		return fmt.Errorf("不支持的水印模式: %s（支持: position, repeat）", wmMode)
	}

	if err != nil {
		return fmt.Errorf("添加水印失败: %w", err)
	}

	fmt.Printf("水印添加完成: %s\n", outputPath)
	return nil
}
