package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"imagetoolbox/internal/watermark"
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

var watermarkCmd = &cobra.Command{
	Use:   "watermark",
	Short: "为图片添加水印",
	Long: `为图片添加文字水印，支持两种模式：

1. position（默认）: 单点位置水印，在指定位置添加水印
   - 自动根据背景亮度选择黑/白文字
   - 支持指定自定义颜色

2. repeat: 重复平铺水印，文字以平铺方式覆盖整张图片
   - 支持旋转角度和间距调整
   - 需要指定字体文件路径`,
	Example: `  # 位置水印（默认右下角，智能颜色）
  imagetoolbox watermark -i photo.jpg -t "Author"

  # 指定位置和透明度
  imagetoolbox watermark -i photo.png -t "Copyright" --position center --opacity 0.8

  # 重复平铺水印
  imagetoolbox watermark -i photo.png -t "WATERMARK" --mode repeat --font /path/to/font.ttf

  # 指定输出路径
  imagetoolbox watermark -i photo.jpg -t "Author" -o output.jpg`,
	RunE: runWatermark,
}

func init() {
	rootCmd.AddCommand(watermarkCmd)

	watermarkCmd.Flags().StringVarP(&wmInputFile, "input", "i", "", "输入图片文件路径")
	watermarkCmd.Flags().StringVarP(&wmOutputFile, "output", "o", "", "输出图片文件路径（默认在原文件名后加 _watermarked）")
	watermarkCmd.Flags().StringVarP(&wmText, "text", "t", "", "水印文字")
	watermarkCmd.Flags().StringVarP(&wmMode, "mode", "m", "position", "水印模式: position（位置）/ repeat（重复平铺）")
	watermarkCmd.Flags().StringVar(&wmColor, "color", "", "水印颜色（空表示自动选择）")
	watermarkCmd.Flags().IntVar(&wmSpace, "space", 0, "平铺间距（0表示自动计算）")
	watermarkCmd.Flags().IntVar(&wmAngle, "angle", 30, "旋转角度（repeat模式）")
	watermarkCmd.Flags().Float64Var(&wmOpacity, "opacity", 0.5, "透明度 (0~1)")
	watermarkCmd.Flags().StringVar(&wmFontPath, "font", "", "字体文件路径")
	watermarkCmd.Flags().IntVar(&wmFontSize, "font-size", 0, "字体大小（0表示自动计算）")
	watermarkCmd.Flags().StringVar(&wmPosition, "position", "bottom-right", "水印位置: bottom-right/bottom-left/top-right/top-left/center")
	watermarkCmd.Flags().Float64Var(&wmMargin, "margin", 0.04, "边距比例（position模式）")

	watermarkCmd.MarkFlagRequired("input")
	watermarkCmd.MarkFlagRequired("text")
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
			FontSize:    &wmFontSize,
			Color:       &wmColor,
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
